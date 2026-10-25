// Package otelhook is a query hook of gocrud for OpenTelemetry: a span of each query, and the
// db.client.operation.duration metric, by the semantic conventions of databases.
package otelhook

import (
	"context"
	"errors"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/semconv/v1.43.0/dbconv"
	"go.opentelemetry.io/otel/trace"

	"github.com/mikogs/gocrud"
)

const scope = "github.com/mikogs/gocrud/otelhook"

// ErrDialect is returned by New for a dialect other than the dialects of gocrud.
var ErrDialect = errors.New("unknown dialect")

type Options struct {
	// Dialect is the dialect of the CRUD of the hook, gocrud.DialectPostgres or gocrud.DialectSQLite.
	Dialect string
	// TracerProvider and MeterProvider are the providers of the spans and the metrics, or the global ones, of
	// otel.SetTracerProvider and otel.SetMeterProvider, when they are nil.
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
}

type hook struct {
	tracer   trace.Tracer
	duration dbconv.ClientOperationDuration
	system   dbconv.SystemNameAttr
}

// New returns the hook, for gocrud.Options.QueryHook.
func New(options Options) (gocrud.QueryHook, error) {
	var system dbconv.SystemNameAttr
	switch options.Dialect {
	case gocrud.DialectPostgres:
		system = dbconv.SystemNamePostgreSQL
	case gocrud.DialectSQLite:
		system = dbconv.SystemNameSQLite
	default:
		return nil, ErrDialect
	}

	tracerProvider := options.TracerProvider
	if tracerProvider == nil {
		tracerProvider = otel.GetTracerProvider()
	}
	meterProvider := options.MeterProvider
	if meterProvider == nil {
		meterProvider = otel.GetMeterProvider()
	}

	duration, err := dbconv.NewClientOperationDuration(meterProvider.Meter(scope))
	if err != nil {
		return nil, err
	}
	return &hook{tracer: tracerProvider.Tracer(scope), duration: duration, system: system}, nil
}

func (h *hook) BeforeQuery(ctx context.Context, event *gocrud.QueryEvent) context.Context {
	operation := sqlOperation(event.Query)
	name := operation
	if event.Table != "" {
		name += " " + event.Table
	}
	attrs := []attribute.KeyValue{
		semconv.DBSystemNameKey.String(string(h.system)),
		semconv.DBOperationName(operation),
		semconv.DBQueryText(event.Query),
		attribute.String("gocrud.operation", event.Op),
	}
	if event.Table != "" {
		attrs = append(attrs, semconv.DBCollectionName(event.Table))
	}
	ctx, _ = h.tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...))
	return ctx
}

func (h *hook) AfterQuery(ctx context.Context, event *gocrud.QueryEvent) {
	span := trace.SpanFromContext(ctx)
	attrs := []attribute.KeyValue{h.duration.AttrOperationName(sqlOperation(event.Query))}
	if event.Table != "" {
		attrs = append(attrs, h.duration.AttrCollectionName(event.Table))
	}
	if event.Err != nil {
		errorType := semconv.ErrorType(event.Err)
		attrs = append(attrs, errorType)
		span.SetAttributes(errorType)
		span.RecordError(event.Err)
		span.SetStatus(codes.Error, event.Err.Error())
	}
	h.duration.Record(ctx, event.Duration.Seconds(), h.system, attrs...)
	span.End()
}

// sqlOperation returns the first keyword of query, e.g. SELECT, which is the operation of the semantic conventions.
func sqlOperation(query string) string {
	operation, _, _ := strings.Cut(strings.TrimSpace(query), " ")
	return strings.ToUpper(strings.TrimRight(operation, ";"))
}
