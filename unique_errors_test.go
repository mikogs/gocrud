package gocrud

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

type UniqueErrorsWithAVeryLongStructNameForTruncationOfNames struct {
	ID                                 uint64
	AVeryLongFieldNameThatIsAlsoUnique string `crud:"uniq"`
	Email                              string `crud:"uniq"`
	Tenant                             string `crud:"uidx:tenant_slug"`
	Slug                               string `crud:"uidx:tenant_slug"`
	Note                               string
}

type UniqueAdded_V1 struct {
	ID   uint64
	Name string
}

type UniqueAdded struct {
	ID    uint64
	Name  string
	Email *string `crud:"uniq"`
}

func testUniqueErrors(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	type item = UniqueErrorsWithAVeryLongStructNameForTruncationOfNames
	_ = DropTable[item](ctx, crud)
	if err := CreateTable[item](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	saved := &item{AVeryLongFieldNameThatIsAlsoUnique: "a", Email: "a", Tenant: "t", Slug: "s", Note: "n"}
	if err := crud.Save(ctx, saved, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	builder, err := crud.builder(&item{})
	if err != nil {
		t.Fatalf("builder failed: %s", err.Error())
	}
	byHand := "unique_errors_by_hand"
	if crud.dialect == DialectSQLite {
		byHand = builder.TableName() + ".note"
	}
	if _, err := crud.conn.ExecContext(ctx, fmt.Sprintf(`CREATE UNIQUE INDEX "unique_errors_by_hand" ON %s ("note")`, builder.QuotedTableName())); err != nil {
		t.Fatalf("CREATE UNIQUE INDEX failed: %s", err.Error())
	}

	for name, test := range map[string]struct {
		obj        *item
		wantField  string
		wantFields []string
	}{
		"truncated name": {&item{AVeryLongFieldNameThatIsAlsoUnique: "a", Email: "b", Slug: "b", Note: "b"},
			"AVeryLongFieldNameThatIsAlsoUnique", []string{"AVeryLongFieldNameThatIsAlsoUnique"}},
		"column":        {&item{AVeryLongFieldNameThatIsAlsoUnique: "c", Email: "a", Slug: "c", Note: "c"}, "Email", []string{"Email"}},
		"index":         {&item{AVeryLongFieldNameThatIsAlsoUnique: "d", Email: "d", Tenant: "t", Slug: "s", Note: "d"}, "Tenant", []string{"Tenant", "Slug"}},
		"index by hand": {&item{AVeryLongFieldNameThatIsAlsoUnique: "e", Email: "e", Slug: "e", Note: "n"}, byHand, nil},
	} {
		t.Run(name, func(t *testing.T) {
			err := crud.Save(ctx, test.obj, SaveOptions{})
			var uniqErr *UniqError
			if !errors.As(err, &uniqErr) || uniqErr.Field != test.wantField || !reflect.DeepEqual(uniqErr.Fields, test.wantFields) {
				t.Fatalf("Save returned err %v (%+v), want a UniqError for %s %v", err, uniqErr, test.wantField, test.wantFields)
			}
		})
	}

	other := &item{AVeryLongFieldNameThatIsAlsoUnique: "f", Email: "f", Slug: "f", Note: "f"}
	if err := crud.Save(ctx, other, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
	other.Email = "a"
	err = crud.Save(ctx, other, SaveOptions{NoInsert: true})
	var uniqErr *UniqError
	if !errors.As(err, &uniqErr) || uniqErr.Field != "Email" {
		t.Fatalf("Save of an update returned err %v, want a UniqError for Email", err)
	}

	_ = DropTable[UniqueAdded](ctx, crud)
	if err := CreateTable[UniqueAdded_V1](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	if _, err := Migrate[UniqueAdded](ctx, crud); err != nil {
		t.Fatalf("Migrate failed: %s", err.Error())
	}
	email := "a@example.com"
	if err := crud.Save(ctx, &UniqueAdded{Name: "a", Email: &email}, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
	err = crud.Save(ctx, &UniqueAdded{Name: "b", Email: &email}, SaveOptions{})
	if !errors.As(err, &uniqErr) || uniqErr.Field != "Email" {
		t.Fatalf("Save of a field added by Migrate returned err %v, want a UniqError for Email", err)
	}
}

func TestUniqueErrors(t *testing.T)        { testUniqueErrors(t, testCRUD) }
func TestSQLite_UniqueErrors(t *testing.T) { testUniqueErrors(t, testCRUDSQLite) }

// pqError and pgxError have the fields of the errors of lib/pq and pgx.
type pqError struct {
	Code       string
	Constraint string
}

func (e *pqError) Error() string    { return "pq: " + e.Code }
func (e *pqError) SQLState() string { return e.Code }

type pgxError struct {
	Code           string
	ConstraintName string
}

func (e *pgxError) Error() string    { return "ERROR (SQLSTATE " + e.Code + ")" }
func (e *pgxError) SQLState() string { return e.Code }

type otherFieldError struct {
	Constraint []string
}

func (e *otherFieldError) Error() string    { return "23505" }
func (e *otherFieldError) SQLState() string { return "23505" }

type stateOnlyError string

func (e stateOnlyError) Error() string    { return string(e) }
func (e stateOnlyError) SQLState() string { return string(e) }

func TestDriverErrors(t *testing.T) {
	for _, test := range []struct {
		err        error
		uniqueKey  string
		unique     bool
		foreignKey bool
	}{
		{&pqError{Code: "23505", Constraint: "user_email_key"}, "user_email_key", true, false},
		{fmt.Errorf("wrapped: %w", &pgxError{Code: "23505", ConstraintName: "user_email_key"}), "user_email_key", true, false},
		{&pqError{Code: "23503", Constraint: "order_user_id_fkey"}, "order_user_id_fkey", false, true},
		{&pgxError{Code: "23503"}, "", false, true},
		{stateOnlyError("23505"), "", true, false},
		{&otherFieldError{Constraint: []string{"user_email_key"}}, "", true, false},
		// a message of another error of PostgreSQL is not read as one of SQLite
		{&pqError{Code: "23514", Constraint: "UNIQUE constraint failed: FOREIGN KEY constraint failed"}, "UNIQUE constraint failed: FOREIGN KEY constraint failed", false, false},
		{errors.New("constraint failed: UNIQUE constraint failed: user.email (2067)"), "user.email", true, false},
		{errors.New("UNIQUE constraint failed: item.tenant, item.slug"), "item.tenant, item.slug", true, false},
		{errors.New("constraint failed: FOREIGN KEY constraint failed (787)"), "", false, true},
		{errors.New("connection refused"), "", false, false},
	} {
		key, unique := uniqueViolationKey(test.err)
		if unique != test.unique || unique && key != test.uniqueKey {
			t.Fatalf("uniqueViolationKey(%v) returned %q, %t, want %q, %t", test.err, key, unique, test.uniqueKey, test.unique)
		}
		if got := isForeignKeyViolation(test.err); got != test.foreignKey {
			t.Fatalf("isForeignKeyViolation(%v) returned %t, want %t", test.err, got, test.foreignKey)
		}
	}
}
