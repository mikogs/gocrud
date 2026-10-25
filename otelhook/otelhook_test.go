package otelhook

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	_ "modernc.org/sqlite"

	"github.com/mikogs/gocrud"
)

type Note struct {
	ID   uint64
	Text string `crud:"uniq"`
}

func TestHook(t *testing.T) {
	ctx := context.Background()
	spans := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	hook, err := New(Options{Dialect: gocrud.DialectSQLite, TracerProvider: tracerProvider, MeterProvider: meterProvider})
	if err != nil {
		t.Fatalf("New failed: %s", err.Error())
	}
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %s", err.Error())
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	crud := gocrud.New(db, gocrud.Options{Dialect: gocrud.DialectSQLite, QueryHook: hook})
	if err := gocrud.CreateTable[Note](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	parentCtx, parent := tracerProvider.Tracer("test").Start(ctx, "request")
	if err := crud.Save(parentCtx, &Note{Text: "a"}, gocrud.SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
	if err := crud.Save(parentCtx, &Note{Text: "a"}, gocrud.SaveOptions{}); err == nil {
		t.Fatal("Save of a duplicate did not fail")
	}
	parent.End()

	ended := spans.Ended()
	var insert, failed sdktrace.ReadOnlySpan
	for _, span := range ended {
		if span.Name() == "INSERT note" && span.Status().Code == codes.Error {
			failed = span
		} else if span.Name() == "INSERT note" {
			insert = span
		}
	}
	if insert == nil || failed == nil {
		names := []string{}
		for _, span := range ended {
			names = append(names, span.Name())
		}
		t.Fatalf("the spans are %v, want an INSERT note and a failed one", names)
	}
	want := map[attribute.Key]string{
		"db.system.name":     "sqlite",
		"db.operation.name":  "INSERT",
		"db.collection.name": "note",
		"gocrud.operation":   "Save",
	}
	got := map[attribute.Key]string{}
	for _, attr := range insert.Attributes() {
		got[attr.Key] = attr.Value.String()
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("the span has %s=%q, want %q, of %v", key, got[key], value, got)
		}
	}
	if insert.SpanKind() != trace.SpanKindClient || insert.Parent().SpanID() != parent.SpanContext().SpanID() || got["db.query.text"] == "" {
		t.Fatalf("the span is of the kind %v, with the parent %v and the query %q, want a client span of the request with the query",
			insert.SpanKind(), insert.Parent().SpanID(), got["db.query.text"])
	}

	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &metrics); err != nil {
		t.Fatalf("Collect failed: %s", err.Error())
	}
	histogram, ok := findMetric(metrics, "db.client.operation.duration").(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("the metrics %v have no histogram db.client.operation.duration", metrics)
	}
	var withError, inserts uint64
	for _, point := range histogram.DataPoints {
		if value, ok := point.Attributes.Value("db.operation.name"); ok && value.AsString() == "INSERT" {
			inserts += point.Count
			if _, ok := point.Attributes.Value("error.type"); ok {
				withError += point.Count
			}
		}
	}
	if inserts != 2 || withError != 1 {
		t.Fatalf("the histogram counted %d INSERT queries, %d of them with an error, want 2 and 1", inserts, withError)
	}

	if _, err := New(Options{Dialect: "mysql"}); !errors.Is(err, ErrDialect) {
		t.Fatalf("New with another dialect returned %v, want ErrDialect", err)
	}
}

func findMetric(metrics metricdata.ResourceMetrics, name string) metricdata.Aggregation {
	for _, scope := range metrics.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == name {
				return m.Data
			}
		}
	}
	return nil
}

func TestSQLOperation(t *testing.T) {
	for query, want := range map[string]string{
		`SELECT "id" FROM "note";`: "SELECT",
		"  insert into note":       "INSERT",
		"BEGIN":                    "BEGIN",
		"COMMIT;":                  "COMMIT",
	} {
		if got := sqlOperation(query); got != want {
			t.Fatalf("sqlOperation(%q) returned %q, want %q", query, got, want)
		}
	}
}
