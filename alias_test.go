package gocrud

import (
	"context"
	"reflect"
	"testing"

	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
)

type AliasProduct struct {
	ID   uint64
	Name string
}

type AliasPrice struct {
	ID             uint64
	AliasProductID uint64
	Name           string
	Price          int64
}

type AliasProduct_WithPrice struct {
	ID        uint64 `crud:"name:t1.id"`
	Name      string `crud:"name:t1.name"`
	PriceName string `crud:"name:t2.name"`
	Price     int64  `crud:"name:t2.price"`
}

const aliasProductWithPriceFrom = `FROM "alias_product" t1 INNER JOIN "alias_price" t2 ON t1.id=t2.alias_product_id`

func (p *AliasProduct_WithPrice) BuildSelectPrefixQuery(tableNamePrefix string) (string, error) {
	return `SELECT t1.id, t1.name, t2.name, t2.price ` + aliasProductWithPriceFrom, nil
}

func (p *AliasProduct_WithPrice) BuildSelectCountPrefixQuery(tableNamePrefix string) (string, error) {
	return `SELECT COUNT(*) AS cnt ` + aliasProductWithPriceFrom, nil
}

func createAliasTestData(t *testing.T, crud *CRUD) {
	t.Helper()
	ctx := context.Background()

	for _, obj := range []interface{}{&AliasProduct{}, &AliasPrice{}} {
		_ = crud.DropTable(ctx, obj)
		if err := crud.CreateTable(ctx, obj); err != nil {
			t.Fatalf("CreateTable failed: %s", err.Error())
		}
	}

	prices := map[string][]AliasPrice{
		"Apple":  {{Name: "small", Price: 10}, {Name: "big", Price: 30}},
		"Banana": {{Name: "small", Price: 20}, {Name: "big", Price: 40}},
		"Cherry": {{Name: "small", Price: 5}},
	}
	for _, productName := range []string{"Apple", "Banana", "Cherry"} {
		product := &AliasProduct{Name: productName}
		if err := crud.Save(ctx, product, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}

		for _, price := range prices[productName] {
			price.AliasProductID = product.ID
			if err := crud.Save(ctx, &price, SaveOptions{}); err != nil {
				t.Fatalf("Save failed: %s", err.Error())
			}
		}
	}
}

func fatalIfNotAliasProductsWithPrice(t *testing.T, crud *CRUD, filters *sqlfilters.Filters, want []string) {
	t.Helper()

	objs, _, err := crud.Get(context.Background(), func() interface{} {
		return &AliasProduct_WithPrice{}
	}, GetOptions{
		Order:   []string{"Price", "desc"},
		Filters: filters,
	})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}

	got := make([]string, 0, len(objs))
	for _, obj := range objs {
		p := obj.(*AliasProduct_WithPrice)
		got = append(got, p.Name+" "+p.PriceName)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Get returned %v, want %v", got, want)
	}

	cnt, err := crud.GetCount(context.Background(), &AliasProduct_WithPrice{}, GetCountOptions{Filters: filters})
	if err != nil {
		t.Fatalf("GetCount failed: %s", err.Error())
	}
	if cnt != uint64(len(want)) {
		t.Fatalf("GetCount returned %d, want %d", cnt, len(want))
	}
}

func testAliasedColumnNames(t *testing.T, crud *CRUD) {
	createAliasTestData(t, crud)

	fatalIfNotAliasProductsWithPrice(t, crud, &sqlfilters.Filters{
		"PriceName": {Op: sqlfilters.OpEqual, Val: "big"},
	}, []string{"Banana big", "Apple big"})

	fatalIfNotAliasProductsWithPrice(t, crud, &sqlfilters.Filters{
		"Name":      {Op: sqlfilters.OpEqual, Val: "Apple"},
		"PriceName": {Op: sqlfilters.OpEqual, Val: "small"},
	}, []string{"Apple small"})

	fatalIfNotAliasProductsWithPrice(t, crud, &sqlfilters.Filters{
		"PriceName": {Op: sqlfilters.OpEqual, Val: "small"},
		sqlfilters.RawKey: {Op: sqlfilters.OpAND, Val: []interface{}{
			".Price > ?",
			8,
		}},
	}, []string{"Banana small", "Apple small"})
}

func TestGet_AliasedColumnNames(t *testing.T) {
	testAliasedColumnNames(t, testCRUD)
}

func TestSQLite_Get_AliasedColumnNames(t *testing.T) {
	testAliasedColumnNames(t, testCRUDSQLite)
}
