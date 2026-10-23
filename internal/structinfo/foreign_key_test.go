package structinfo

import (
	"errors"
	"reflect"
	"testing"
)

func TestNew_ForeignKeys(t *testing.T) {
	type order struct {
		ID          uint64
		CustomerID  uint64  `crud:"idx fk:customer on_delete:cascade"`
		ShipperID   *uint64 `crud:"fk:shipper on_delete:set_null"`
		WarehouseID uint64  `crud:"fk:warehouse on_delete:restrict"`
		NoteID      uint64  `crud:"fk:note"`
	}

	info := mustNew(t, &order{}, "crud")
	if !reflect.DeepEqual(info.ForeignKeyFields, []string{"CustomerID", "ShipperID", "WarehouseID", "NoteID"}) {
		t.Fatalf("New returned foreign key fields %v", info.ForeignKeyFields)
	}

	want := map[string][2]string{
		"CustomerID":  {"customer", OnDeleteCascade},
		"ShipperID":   {"shipper", OnDeleteSetNull},
		"WarehouseID": {"warehouse", OnDeleteRestrict},
		"NoteID":      {"note", ""},
	}
	for field, foreignKey := range want {
		if got := [2]string{info.Fields[field].ForeignKeyTable, info.Fields[field].OnDelete}; got != foreignKey {
			t.Fatalf("New returned foreign key %v for %s, want %v", got, field, foreignKey)
		}
	}
	if !info.Fields["CustomerID"].Index {
		t.Fatal("New did not keep the other options of a foreign key field")
	}
}

func TestNew_InvalidForeignKeys(t *testing.T) {
	tests := map[string]interface{}{
		"on_delete without fk": &struct {
			ID         uint64
			CustomerID uint64 `crud:"on_delete:cascade"`
		}{},
		"invalid table name": &struct {
			ID         uint64
			CustomerID uint64 `crud:"fk:Customer"`
		}{},
		"on ID": &struct {
			ID   uint64 `crud:"fk:customer"`
			Name string
		}{},
		"on an int64": &struct {
			ID   uint64
			Code int64 `crud:"fk:customer"`
		}{},
		"on a signed integer": &struct {
			ID         uint64
			CustomerID int64 `crud:"fk:customer"`
		}{},
		"set_null on a field that is not nullable": &struct {
			ID         uint64
			CustomerID uint64 `crud:"fk:customer on_delete:set_null"`
		}{},
		"unknown action": &struct {
			ID         uint64
			CustomerID uint64 `crud:"fk:customer on_delete:ignore"`
		}{},
	}

	for name, obj := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := New(obj, "crud"); !errors.Is(err, ErrForeignKey) {
				t.Fatalf("New returned err %v, want %v", err, ErrForeignKey)
			}
		})
	}
}
