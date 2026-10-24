package dialect

import (
	"database/sql/driver"
	"strings"
	"testing"
	"time"
)

func TestLiteral(t *testing.T) {
	at := time.Date(2024, 5, 1, 12, 30, 0, 500000000, time.FixedZone("CEST", 2*60*60))
	tests := []struct {
		value            driver.Value
		postgres, sqlite string
	}{
		{nil, "NULL", "NULL"},
		{int64(-7), "-7", "-7"},
		{1.5, "1.5", "1.5"},
		{true, "true", "1"},
		{false, "false", "0"},
		{"it's", "'it''s'", "'it''s'"},
		{[]byte{0xab, 0x01}, `'\xab01'`, "X'ab01'"},
		{at, "'2024-05-01 10:30:00.5+00'", "'2024-05-01 10:30:00.500000000+00:00'"},
	}
	for _, test := range tests {
		if got := (Postgres{}).Literal(test.value); got != test.postgres {
			t.Fatalf("Postgres.Literal(%v) returned %s, want %s", test.value, got, test.postgres)
		}
		if got := (SQLite{}).Literal(test.value); got != test.sqlite {
			t.Fatalf("SQLite.Literal(%v) returned %s, want %s", test.value, got, test.sqlite)
		}
	}
}

func TestUniqueKey(t *testing.T) {
	for _, test := range []struct {
		table, name      string
		columns          []string
		postgres, sqlite string
	}{
		{"user", "", []string{"email"}, "user_email_key", "user.email"},
		{"item", "idx_item_tenant_slug", []string{"tenant", "slug"}, "idx_item_tenant_slug", "item.tenant, item.slug"},
		{"t", "idx_" + strings.Repeat("a", 70), []string{"a"}, "idx_" + strings.Repeat("a", 59), "t.a"},
		// the longer name is shortened first, until the name has 63 bytes
		{strings.Repeat("t", 40), "", []string{strings.Repeat("c", 30)}, strings.Repeat("t", 29) + "_" + strings.Repeat("c", 29) + "_key", ""},
		{strings.Repeat("t", 60), "", []string{"email"}, strings.Repeat("t", 53) + "_email_key", ""},
		// a character that does not fit is left out
		{strings.Repeat("t", 52) + "ł", "", []string{"email"}, strings.Repeat("t", 52) + "_email_key", ""},
	} {
		if got := (Postgres{}).UniqueKey(test.table, test.name, test.columns); got != test.postgres {
			t.Fatalf("Postgres.UniqueKey(%s, %s, %v) returned %s, want %s", test.table, test.name, test.columns, got, test.postgres)
		}
		if got := (SQLite{}).UniqueKey(test.table, test.name, test.columns); test.sqlite != "" && got != test.sqlite {
			t.Fatalf("SQLite.UniqueKey(%s, %s, %v) returned %s, want %s", test.table, test.name, test.columns, got, test.sqlite)
		}
	}
}
