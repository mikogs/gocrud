package registry

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
	}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})

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

	objs, _, err := svc.List(ctx, "contacts", ListOptions{Limit: 10, Order: "ID", OrderDirection: "asc", Filters: filters})
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
			Vals: map[string]string{"Name~q": "%acme%", "Email~q": "%acme%"},
			Ops:  map[string]string{"Name~q": "like", "Email~q": "like"},
		}, []uint64{1, 2, 4}},
		"range on one field": {Filters{
			Vals: map[string]string{"Age~min": "35", "Age~max": "45"},
			Ops:  map[string]string{"Age~min": "ge", "Age~max": "le"},
		}, []uint64{2, 4}},
		"groups and conditions": {Filters{
			Vals: map[string]string{"OwnerID": "1", "Name~q": "Carol", "Age~q": "30", "Email~x": "%example.org"},
			Ops:  map[string]string{"Age~q": "le", "Email~x": "like"},
		}, []uint64{1}},
		"operator defaults to eq": {Filters{
			Vals: map[string]string{"Name~q": "Bob", "Age~q": "50"},
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
			Vals:      map[string]string{"Name~q": "%acme%", "Email~q": "%acme%"},
			Ops:       map[string]string{"Name~q": "like", "Email~q": "like"},
			ScopeVals: scope,
		}, []uint64{1, 2}},
		"group cannot widen the scope": {Filters{
			Vals:      map[string]string{"OwnerID~q": "2", "Name~q": "Bob"},
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
		"empty group":          {Filters{Vals: map[string]string{"Name~": "Bob"}}, map[string]string{"Name~": gocrud.ViolationType}},
		"empty field":          {Filters{Vals: map[string]string{"~q": "Bob"}}, map[string]string{"~q": gocrud.ViolationType}},
		"group with a tilde":   {Filters{Vals: map[string]string{"Name~q~x": "Bob"}}, map[string]string{"Name~q~x": gocrud.ViolationType}},
		"empty part of a path": {Filters{Vals: map[string]string{"Owner..Name": "Bob"}}, map[string]string{"Owner..Name": gocrud.ViolationType}},
		"group in the scope":   {Filters{ScopeVals: map[string]string{"OwnerID~q": "1"}}, map[string]string{"OwnerID~q": gocrud.ViolationType}},
		"invalid op in group":  {Filters{Vals: map[string]string{"Age~q": "1"}, Ops: map[string]string{"Age~q": "bogus"}}, map[string]string{"Age": gocrud.ViolationType}},
		"invalid value in group": {Filters{
			Vals: map[string]string{"Name~q": "Bob", "Age~q": "200"},
		}, map[string]string{"Age": "max"}},
		"invalid op in scope": {Filters{ScopeVals: map[string]string{"OwnerID": "1"}, ScopeOps: map[string]string{"OwnerID": "bogus"}}, map[string]string{"OwnerID": gocrud.ViolationType}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := svc.List(context.Background(), "contacts", ListOptions{Limit: 10, Filters: test.filters})
			fatalIfNotFilterViolations(t, err, test.want)
		})
	}
}

type Account struct {
	ID       uint64
	Name     string
	Country  string
	Projects []*Project `crud:"rel:AccountID"`
}

type Project struct {
	ID        uint64
	AccountID uint64
	Name      string
	Account   *Account `crud:"rel:AccountID"`
}

func TestList_FilterPaths(t *testing.T) {
	ctx := context.Background()
	svc := New(map[string]func() interface{}{
		"accounts": func() interface{} { return &Account{} },
		"projects": func() interface{} { return &Project{} },
	}, testDB, gocrud.Options{Dialect: gocrud.DialectPostgres})

	_ = testCRUD.DropTable(ctx, &Project{})
	_ = testCRUD.DropTable(ctx, &Account{})
	if err := svc.CreateTables(ctx); err != nil {
		t.Fatalf("CreateTables failed: %s", err.Error())
	}
	for _, obj := range []interface{}{
		&Account{Name: "Acme", Country: "PL"}, &Account{Name: "Globex", Country: "DE"},
		&Project{AccountID: 1, Name: "Website"}, &Project{AccountID: 2, Name: "Acme clone"}, &Project{AccountID: 1, Name: "App"},
	} {
		if err := svc.Save(ctx, obj, nil); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	projects := map[string]struct {
		filters   Filters
		order     string
		direction string
		want      []uint64
	}{
		"belongs to": {Filters{Vals: map[string]string{"Account.Country": "PL"}}, "ID", "asc", []uint64{1, 3}},
		"in a group": {Filters{
			Vals: map[string]string{"Name~q": "%Acme%", "Account.Name~q": "%Acme%"},
			Ops:  map[string]string{"Name~q": "like", "Account.Name~q": "like"},
		}, "ID", "asc", []uint64{1, 2, 3}},
		"in the scope": {Filters{
			Vals:      map[string]string{"Name": "Acme clone"},
			ScopeVals: map[string]string{"Account.Country": "PL"},
		}, "ID", "asc", []uint64{}},
		// the projects of the same account are ordered by ID, in the direction of the order
		"ordered by a related field": {Filters{}, "Account.Name", "desc", []uint64{2, 3, 1}},
	}
	for name, test := range projects {
		t.Run("projects "+name, func(t *testing.T) {
			objs, _, err := svc.List(ctx, "projects", ListOptions{Limit: 10, Order: test.order, OrderDirection: test.direction, Filters: test.filters})
			if err != nil {
				t.Fatalf("List failed: %s", err.Error())
			}
			ids := []uint64{}
			for _, obj := range objs {
				ids = append(ids, obj.(*Project).ID)
			}
			if !reflect.DeepEqual(ids, test.want) {
				t.Fatalf("List returned projects %v, want %v", ids, test.want)
			}
		})
	}

	num, err := svc.Num(ctx, "accounts", Filters{Vals: map[string]string{"Projects.Name": "App"}})
	if err != nil || num != 1 {
		t.Fatalf("Num returned %d, %v, want the account with the project", num, err)
	}
}
