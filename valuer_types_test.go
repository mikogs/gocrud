package gocrud

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Money is an amount in cents, written as text with two decimal places, as decimal types are.
type Money struct {
	cents int64
}

func (m Money) Value() (driver.Value, error) {
	return fmt.Sprintf("%d.%02d", m.cents/100, m.cents%100), nil
}

func (m *Money) Scan(src any) error {
	var text string
	switch v := src.(type) {
	case string:
		text = v
	case []byte:
		text = string(v)
	case float64:
		text = strconv.FormatFloat(v, 'f', 2, 64)
	case int64:
		m.cents = v * 100
		return nil
	default:
		return fmt.Errorf("cannot scan %T into Money", src)
	}

	whole, fraction, _ := strings.Cut(text, ".")
	fraction = (fraction + "00")[:2]
	cents, err := strconv.ParseInt(whole+fraction, 10, 64)
	m.cents = cents
	return err
}

// Signature is written as bytes, which start with a version also in its zero value.
type Signature struct {
	data []byte
}

func (s Signature) Value() (driver.Value, error) { return append([]byte{0xab}, s.data...), nil }

func (s *Signature) Scan(src any) error {
	data, ok := src.([]byte)
	if !ok || len(data) == 0 || data[0] != 0xab {
		return fmt.Errorf("%v is not a signature", src)
	}
	s.data = bytes.Clone(data[1:])
	return nil
}

type ValuerItem struct {
	ID       uint64
	Name     string
	Price    Money `crud:"type:numeric(12,2)"`
	Fee      Money
	Discount *Money
	Note     sql.NullString
	Count    sql.NullInt64
	Ref      uuid.UUID
	Image    []byte
	Thumb    *[]byte
	Raw      json.RawMessage
	Sig      Signature
}

// ValuerItem_Text is ValuerItem with columns of text instead of the numeric of Price and the bytes of Image
type ValuerItem_Text struct {
	ID    uint64
	Name  string
	Price string `crud:"type:text"`
	Image string `crud:"type:text"`
}

// ValuerItem_V1 is the table of ValuerItem before the columns of Valuer types and bytes were added
type ValuerItem_V1 struct {
	ID   uint64
	Name string
}

func testValuerTypes(t *testing.T, crud *CRUD) {
	ctx := context.Background()
	_ = DropTable[ValuerItem](ctx, crud)
	if err := CreateTable[ValuerItem_V1](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	if err := crud.Save(ctx, &ValuerItem_V1{Name: "old"}, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	migration, err := Migrate[ValuerItem](ctx, crud)
	if err != nil || len(migration.AddedColumns) != 10 || len(migration.Differences) > 0 {
		t.Fatalf("Migrate returned %+v, %v, want 10 added columns", migration, err)
	}
	if err := crud.CheckSchema(ctx, &ValuerItem{}); err != nil {
		t.Fatalf("CheckSchema failed: %s", err.Error())
	}

	old, err := Load[ValuerItem](ctx, crud, 1)
	if err != nil || old.Price.cents != 0 || old.Fee.cents != 0 || old.Note.Valid || old.Ref != uuid.Nil || len(old.Image) != 0 ||
		old.Thumb != nil || len(old.Sig.data) != 0 {
		t.Fatalf("Load returned %+v, %v, want the zero values in the added columns", old, err)
	}

	discount := Money{cents: 250}
	thumb := []byte{0, 1, 2}
	item := &ValuerItem{
		Name:     "new",
		Price:    Money{cents: 123456},
		Fee:      Money{cents: 5},
		Discount: &discount,
		Note:     sql.NullString{String: "it's", Valid: true},
		Count:    sql.NullInt64{Int64: 7, Valid: true},
		Ref:      uuid.MustParse("0190c4a2-7b3e-7cc1-9a3f-1d2e3f405162"),
		Image:    []byte("image"),
		Thumb:    &thumb,
		Raw:      json.RawMessage(`{"a":1}`),
		Sig:      Signature{data: []byte{1, 2}},
	}
	if err := crud.Save(ctx, item, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	loaded, err := Load[ValuerItem](ctx, crud, item.ID)
	if err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	if loaded.Price != item.Price || loaded.Fee != item.Fee || *loaded.Discount != discount || loaded.Note != item.Note ||
		loaded.Count != item.Count || loaded.Ref != item.Ref || !bytes.Equal(loaded.Image, item.Image) ||
		!bytes.Equal(*loaded.Thumb, thumb) || !bytes.Equal(loaded.Raw, item.Raw) || !bytes.Equal(loaded.Sig.data, item.Sig.data) {
		t.Fatalf("Load returned %+v, want %+v", loaded, item)
	}

	loaded.Discount, loaded.Note, loaded.Image, loaded.Thumb = nil, sql.NullString{}, nil, nil
	if err := crud.Save(ctx, loaded, SaveOptions{}); err != nil {
		t.Fatalf("Save of NULLs and empty bytes failed: %s", err.Error())
	}

	err = UpdateMultiple[ValuerItem](ctx, crud, map[string]any{"Raw": json.RawMessage(nil), "Count": sql.NullInt64{}}, UpdateMultipleOptions{})
	if err != nil {
		t.Fatalf("UpdateMultiple of NULLs and empty bytes failed: %s", err.Error())
	}

	items, err := Get[ValuerItem](ctx, crud, GetOptions{Order: []string{"ID", "asc"}})
	if err != nil || len(items) != 2 {
		t.Fatalf("Get returned %d items, %v, want 2", len(items), err)
	}
	if got := items[1]; got.Discount != nil || got.Note.Valid || len(got.Image) != 0 || got.Thumb != nil || len(got.Raw) != 0 ||
		got.Count.Valid || got.Price != item.Price {
		t.Fatalf("Get returned %+v, want NULLs and empty bytes", got)
	}

	_ = DropTable[ValuerItem](ctx, crud)
	if err := CreateTable[ValuerItem_Text](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}
	planned, err := PlanMigration[ValuerItem](ctx, crud)
	if err != nil {
		t.Fatalf("PlanMigration failed: %s", err.Error())
	}
	var typeDifferences []string
	for _, difference := range planned.Differences {
		if strings.Contains(difference, " has type ") {
			typeDifferences = append(typeDifferences, difference)
		}
	}
	if len(typeDifferences) != 2 || !strings.HasPrefix(typeDifferences[0], `column "price"`) || !strings.HasPrefix(typeDifferences[1], `column "image"`) {
		t.Fatalf("PlanMigration returned the differences of types %v, want the ones of price and image", typeDifferences)
	}
}

func TestValuerTypes(t *testing.T)        { testValuerTypes(t, testCRUD) }
func TestSQLite_ValuerTypes(t *testing.T) { testValuerTypes(t, testCRUDSQLite) }
