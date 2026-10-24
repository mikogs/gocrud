package gocrud

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mikogs/gocrud/internal/structinfo"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type Article struct {
	ID          uint64
	Title       string `validate:"required"`
	Views       int64
	PublishedAt *time.Time
}

func createArticles(t *testing.T, crud *CRUD) []*Article {
	t.Helper()
	ctx := context.Background()

	_ = DropTable[Article](ctx, crud)
	if err := CreateTable[Article](ctx, crud); err != nil {
		t.Fatalf("CreateTable failed: %s", err.Error())
	}

	publishedAt := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	articles := []*Article{
		{Title: "Generics in Go", Views: 120, PublishedAt: &publishedAt},
		{Title: "Transactions", Views: 80, PublishedAt: &publishedAt},
		{Title: "Draft"},
	}
	for _, article := range articles {
		if err := crud.Save(ctx, article, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	return articles
}

func titles(articles []*Article) []string {
	got := make([]string, len(articles))
	for i, article := range articles {
		got[i] = article.Title
	}
	return got
}

func testGenericGet(t *testing.T, crud *CRUD) {
	createArticles(t, crud)

	articles, err := Get[Article](context.Background(), crud, GetOptions{
		Order:   []string{"Views", "desc"},
		Filters: &sqlfilters.Filters{"PublishedAt": {Op: sqlfilters.OpNotEqual, Val: nil}},
	})
	if err != nil {
		t.Fatalf("Get failed: %s", err.Error())
	}
	if got := titles(articles); len(got) != 2 || got[0] != "Generics in Go" || got[1] != "Transactions" {
		t.Fatalf("Get returned %v, want [Generics in Go Transactions]", got)
	}
	if articles[0].Views != 120 || articles[0].PublishedAt == nil {
		t.Fatalf("Get returned %+v, want all the fields", *articles[0])
	}

	none, err := Get[Article](context.Background(), crud, GetOptions{
		Filters: &sqlfilters.Filters{"Views": {Op: sqlfilters.OpGreater, Val: 1000}},
	})
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("Get returned %v, %v, want an empty slice", none, err)
	}

	_, err = Get[Article](context.Background(), crud, GetOptions{
		Filters:                  &sqlfilters.Filters{"Views": {Op: sqlfilters.OpGreater, Val: "many"}},
		ConvertFiltersFromString: true,
	})
	fatalIfNotViolations(t, err, map[string]string{"Views": ViolationType})
}

func testGenericLoad(t *testing.T, crud *CRUD) {
	articles := createArticles(t, crud)

	article, err := Load[Article](context.Background(), crud, articles[1].ID)
	if err != nil {
		t.Fatalf("Load failed: %s", err.Error())
	}
	if article.Title != "Transactions" || article.Views != 80 {
		t.Fatalf("Load returned %+v, want the Transactions article", *article)
	}

	article, err = Load[Article](context.Background(), crud, 999)
	if !errors.Is(err, ErrNotFound) || article != nil {
		t.Fatalf("Load returned %v, %v, want nil, %v", article, err, ErrNotFound)
	}
}

func testGenericCountUpdateDelete(t *testing.T, crud *CRUD) {
	createArticles(t, crud)
	ctx := context.Background()
	drafts := &sqlfilters.Filters{"PublishedAt": {Op: sqlfilters.OpEqual, Val: nil}}

	cnt, err := GetCount[Article](ctx, crud, GetCountOptions{Filters: drafts})
	if err != nil || cnt != 1 {
		t.Fatalf("GetCount returned %d, %v, want 1", cnt, err)
	}

	publishedAt := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	err = UpdateMultiple[Article](ctx, crud, map[string]any{"PublishedAt": publishedAt}, UpdateMultipleOptions{Filters: drafts})
	if err != nil {
		t.Fatalf("UpdateMultiple failed: %s", err.Error())
	}
	if cnt, _ := GetCount[Article](ctx, crud, GetCountOptions{Filters: drafts}); cnt != 0 {
		t.Fatalf("GetCount returned %d drafts after UpdateMultiple, want 0", cnt)
	}

	_, err = DeleteMultiple[Article](ctx, crud, DeleteMultipleOptions{
		Filters: &sqlfilters.Filters{"Views": {Op: sqlfilters.OpLower, Val: 100}},
	})
	if err != nil {
		t.Fatalf("DeleteMultiple failed: %s", err.Error())
	}
	if cnt, _ := GetCount[Article](ctx, crud, GetCountOptions{}); cnt != 1 {
		t.Fatalf("GetCount returned %d after DeleteMultiple, want 1", cnt)
	}
}

func testGenericInTx(t *testing.T, crud *CRUD) {
	createArticles(t, crud)
	ctx := context.Background()
	errAbort := errors.New("abort")

	err := crud.RunInTx(ctx, func(tx *CRUD) error {
		if err := tx.Save(ctx, &Article{Title: "Uncommitted"}, SaveOptions{}); err != nil {
			return err
		}

		articles, err := Get[Article](ctx, tx, GetOptions{Order: []string{"ID", "asc"}})
		if err != nil {
			return err
		}
		if len(articles) != 4 || articles[3].Title != "Uncommitted" {
			t.Fatalf("Get in the transaction returned %v, want the uncommitted article", titles(articles))
		}

		if _, err := Load[Article](ctx, tx, articles[3].ID); err != nil {
			t.Fatalf("Load in the transaction failed: %s", err.Error())
		}
		return errAbort
	})
	if !errors.Is(err, errAbort) {
		t.Fatalf("RunInTx returned %v, want %v", err, errAbort)
	}

	if cnt, _ := GetCount[Article](ctx, crud, GetCountOptions{}); cnt != 3 {
		t.Fatalf("GetCount returned %d after a rollback, want 3", cnt)
	}
}

func testGenericInvalidUse(t *testing.T, crud *CRUD) {
	createArticles(t, crud)
	ctx := context.Background()

	for name, options := range map[string]GetOptions{
		"RowObjTransformFunc": {RowObjTransformFunc: func(obj any) any { return obj }},
	} {
		if _, err := Get[Article](ctx, crud, options); !errors.Is(err, ErrUnsupportedOption) {
			t.Fatalf("Get with %s returned err %v, want %v", name, err, ErrUnsupportedOption)
		}
	}

	if _, err := Get[*Article](ctx, crud, GetOptions{}); !errors.Is(err, structinfo.ErrNotStructPointer) {
		t.Fatalf("Get with a pointer type returned err %v, want %v", err, structinfo.ErrNotStructPointer)
	}
	if _, err := Load[int](ctx, crud, 1); !errors.Is(err, structinfo.ErrNotStructPointer) {
		t.Fatalf("Load with a type that is not a struct returned err %v, want %v", err, structinfo.ErrNotStructPointer)
	}
}

func TestGeneric_Get(t *testing.T)        { testGenericGet(t, testCRUD) }
func TestSQLite_Generic_Get(t *testing.T) { testGenericGet(t, testCRUDSQLite) }

func TestGeneric_Load(t *testing.T)        { testGenericLoad(t, testCRUD) }
func TestSQLite_Generic_Load(t *testing.T) { testGenericLoad(t, testCRUDSQLite) }

func TestGeneric_CountUpdateDelete(t *testing.T) { testGenericCountUpdateDelete(t, testCRUD) }
func TestSQLite_Generic_CountUpdateDelete(t *testing.T) {
	testGenericCountUpdateDelete(t, testCRUDSQLite)
}

func TestGeneric_InTx(t *testing.T)        { testGenericInTx(t, testCRUD) }
func TestSQLite_Generic_InTx(t *testing.T) { testGenericInTx(t, testCRUDSQLite) }

func TestGeneric_InvalidUse(t *testing.T)        { testGenericInvalidUse(t, testCRUD) }
func TestSQLite_Generic_InvalidUse(t *testing.T) { testGenericInvalidUse(t, testCRUDSQLite) }
