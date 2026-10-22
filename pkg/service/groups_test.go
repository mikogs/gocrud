package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/mikogs/gocrud"
)

type Contact struct {
	ID      uint64
	OwnerID uint64
	Name    string
	Email   string
	Age     int64 `validate:"max=150"`
}

func contactsService(t *testing.T) *CRUD {
	t.Helper()
	ctx := context.Background()

	svc := New(map[string]func() interface{}{
		"contacts": func() interface{} { return &Contact{} },
	}, testDB, gocrud.DialectPostgres)

	_ = testCRUD.DropTable(ctx, &Contact{})
	if err := svc.CreateTables(ctx); err != nil {
		t.Fatalf("CreateTables failed: %s", err.Error())
	}

	for _, contact := range []*Contact{
		{OwnerID: 1, Name: "acme", Email: "office@example.org", Age: 30},
		{OwnerID: 1, Name: "Bob", Email: "bob@acme.com", Age: 40},
		{OwnerID: 1, Name: "Carol", Email: "carol@example.com", Age: 50},
		{OwnerID: 2, Name: "Acme Inc", Email: "acme@example.com", Age: 40},
	} {
		if err := svc.Save(ctx, contact, 0); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	return svc
}

func fatalIfNotContacts(t *testing.T, svc *CRUD, filters Filters, want []uint64) {
	t.Helper()
	ctx := context.Background()

	objs, _, err := svc.List(ctx, "contacts", 10, 0, "ID", "asc", filters, nil, nil, nil)
	if err != nil {
		t.Fatalf("List failed: %s", err.Error())
	}

	got := []uint64{}
	for _, obj := range objs {
		got = append(got, obj.(*Contact).ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List returned contacts %v, want %v", got, want)
	}

	num, err := svc.Num(ctx, "contacts", filters)
	if err != nil || num != uint64(len(want)) {
		t.Fatalf("Num returned %d, %v, want %d", num, err, len(want))
	}
}

func TestList_FilterGroups(t *testing.T) {
	svc := contactsService(t)

	tests := map[string]struct {
		filters Filters
		want    []uint64
	}{
		"search in several fields": {Filters{
			Vals: map[string]string{"Name.q": "%acme%", "Email.q": "%acme%"},
			Ops:  map[string]string{"Name.q": "like", "Email.q": "like"},
		}, []uint64{1, 2, 4}},
		"range on one field": {Filters{
			Vals: map[string]string{"Age.min": "35", "Age.max": "45"},
			Ops:  map[string]string{"Age.min": "ge", "Age.max": "le"},
		}, []uint64{2, 4}},
		"groups and conditions": {Filters{
			Vals: map[string]string{"OwnerID": "1", "Name.q": "Carol", "Age.q": "30", "Email.x": "%example.org"},
			Ops:  map[string]string{"Age.q": "le", "Email.x": "like"},
		}, []uint64{1}},
		"operator defaults to eq": {Filters{
			Vals: map[string]string{"Name.q": "Bob", "Age.q": "50"},
		}, []uint64{2, 3}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fatalIfNotContacts(t, svc, test.filters, test.want)
		})
	}
}

func TestList_FilterScope(t *testing.T) {
	svc := contactsService(t)
	scope := map[string]string{"OwnerID": "1"}

	tests := map[string]struct {
		filters Filters
		want    []uint64
	}{
		"scope only": {Filters{ScopeVals: scope}, []uint64{1, 2, 3}},
		"scope and a group": {Filters{
			Vals:      map[string]string{"Name.q": "%acme%", "Email.q": "%acme%"},
			Ops:       map[string]string{"Name.q": "like", "Email.q": "like"},
			ScopeVals: scope,
		}, []uint64{1, 2}},
		"group cannot widen the scope": {Filters{
			Vals:      map[string]string{"OwnerID.q": "2", "Name.q": "Bob"},
			ScopeVals: scope,
		}, []uint64{2}},
		"condition cannot replace the scope": {Filters{
			Vals:      map[string]string{"OwnerID": "2"},
			ScopeVals: scope,
		}, []uint64{}},
		"scope with an operator": {Filters{
			ScopeVals: map[string]string{"OwnerID": "2"},
			ScopeOps:  map[string]string{"OwnerID": "ne"},
		}, []uint64{1, 2, 3}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fatalIfNotContacts(t, svc, test.filters, test.want)
		})
	}
}

func TestList_InvalidFilterGroups(t *testing.T) {
	svc := contactsService(t)

	tests := map[string]struct {
		filters Filters
		want    map[string]string
	}{
		"empty group":         {Filters{Vals: map[string]string{"Name.": "Bob"}}, map[string]string{"Name.": gocrud.ViolationType}},
		"empty field":         {Filters{Vals: map[string]string{".q": "Bob"}}, map[string]string{".q": gocrud.ViolationType}},
		"group with a dot":    {Filters{Vals: map[string]string{"Name.q.x": "Bob"}}, map[string]string{"Name.q.x": gocrud.ViolationType}},
		"invalid op in group": {Filters{Vals: map[string]string{"Age.q": "1"}, Ops: map[string]string{"Age.q": "bogus"}}, map[string]string{"Age": gocrud.ViolationType}},
		"invalid value in group": {Filters{
			Vals: map[string]string{"Name.q": "Bob", "Age.q": "200"},
		}, map[string]string{"Age": "max"}},
		"invalid op in scope": {Filters{ScopeVals: map[string]string{"OwnerID": "1"}, ScopeOps: map[string]string{"OwnerID": "bogus"}}, map[string]string{"OwnerID": gocrud.ViolationType}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := svc.List(context.Background(), "contacts", 10, 0, "", "", test.filters, nil, nil, nil)
			fatalIfNotFilterViolations(t, err, test.want)
		})
	}
}
