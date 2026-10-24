package gocrud

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/mikogs/gocrud/pkg/sqlfilters"
)

type UAuthor struct {
	ID    string
	Name  string
	Posts []*UPost `crud:"rel:AuthorID"`
}

type UPost struct {
	ID       string
	AuthorID string  `crud:"fk:u_author on_delete:cascade"`
	EditorID *string `crud:"fk:u_author on_delete:set_null"`
	Title    string
	Author   *UAuthor   `crud:"rel:AuthorID"`
	Editor   *UAuthor   `crud:"rel:EditorID"`
	Comments []UComment `crud:"rel:PostID"`
}

// UComment has a numeric ID, and references a post with a UUID.
type UComment struct {
	ID     uint64
	PostID string `crud:"fk:u_post on_delete:cascade"`
	Body   string
}

// UReview references a post without a foreign key, so its key is stored as it is given.
type UReview struct {
	ID      uint64
	PostRef string
	Post    *UPost `crud:"rel:PostRef"`
}

type uuidRelationsData struct {
	alice, bob *UAuthor
	posts      []*UPost
}

func createUUIDRelationTables(t *testing.T, crud *CRUD) *uuidRelationsData {
	t.Helper()
	ctx := context.Background()

	for _, obj := range []interface{}{&UReview{}, &UComment{}, &UPost{}, &UAuthor{}} {
		_ = crud.DropTable(ctx, obj)
	}
	for _, obj := range []interface{}{&UAuthor{}, &UPost{}, &UComment{}, &UReview{}} {
		if err := crud.CreateTable(ctx, obj); err != nil {
			t.Fatalf("CreateTable failed for %T: %s", obj, err.Error())
		}
	}

	data := &uuidRelationsData{alice: &UAuthor{Name: "Alice"}, bob: &UAuthor{Name: "Bob"}}
	for _, author := range []*UAuthor{data.alice, data.bob} {
		if err := crud.Save(ctx, author, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	bobID := strings.ToUpper(data.bob.ID)
	data.posts = []*UPost{
		{AuthorID: strings.ToUpper(data.alice.ID), EditorID: &bobID, Title: "first"},
		{AuthorID: data.alice.ID, Title: "second"},
		{AuthorID: data.bob.ID, Title: "third"},
	}
	for _, post := range data.posts {
		if err := crud.Save(ctx, post, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	for _, comment := range []*UComment{{PostID: data.posts[0].ID, Body: "a"}, {PostID: data.posts[0].ID, Body: "b"}} {
		if err := crud.Save(ctx, comment, SaveOptions{}); err != nil {
			t.Fatalf("Save failed: %s", err.Error())
		}
	}

	return data
}

func testUUIDRelationsPreload(t *testing.T, crud *CRUD) {
	data := createUUIDRelationTables(t, crud)
	ctx := context.Background()

	post := data.posts[0]
	if post.AuthorID != data.alice.ID || *post.EditorID != data.bob.ID {
		t.Fatalf("Save set keys %s and %s, want the UUIDs in their standard form", post.AuthorID, *post.EditorID)
	}

	loaded, err := Load[UPost](ctx, crud, post.ID, "Author", "Editor", "Comments")
	if err != nil || loaded.Author == nil || loaded.Author.Name != "Alice" || loaded.Editor == nil || loaded.Editor.Name != "Bob" || len(loaded.Comments) != 2 {
		t.Fatalf("Load returned %+v, %v, want the post with its author, editor and comments", loaded, err)
	}

	review := &UReview{PostRef: strings.ToUpper(post.ID)}
	if err := crud.Save(ctx, review, SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}
	if loaded, err := Load[UReview](ctx, crud, review.ID, "Post"); err != nil || loaded.Post == nil || loaded.Post.ID != post.ID {
		t.Fatalf("Load returned %+v, %v, want the review with the post of its uppercase key", loaded, err)
	}

	authors, err := Get[UAuthor](ctx, crud, GetOptions{Order: []string{"Name", "asc"}, Preload: []string{"Posts.Comments"}})
	if err != nil || len(authors) != 2 || len(authors[0].Posts) != 2 || len(authors[1].Posts) != 1 || len(authors[0].Posts[0].Comments) != 2 {
		t.Fatalf("Get returned %+v, %v, want Alice with 2 posts, and Bob with 1", authors, err)
	}
}

func testUUIDRelationsKeys(t *testing.T, crud *CRUD) {
	data := createUUIDRelationTables(t, crud)
	ctx := context.Background()

	err := crud.Save(ctx, &UPost{AuthorID: uuid.NewString(), Title: "orphan"}, SaveOptions{})
	if !errors.Is(err, ErrForeignKeyViolation) {
		t.Fatalf("Save of a post of an author that does not exist returned %v, want %v", err, ErrForeignKeyViolation)
	}

	err = crud.Save(ctx, &UPost{AuthorID: "", Title: "no author"}, SaveOptions{})
	fatalIfNotViolations(t, err, map[string]string{"AuthorID": ViolationType})

	posts, err := Get[UPost](ctx, crud, GetOptions{
		Order:   []string{"Title", "asc"},
		Filters: sqlfilters.Filters{"AuthorID": {Op: sqlfilters.OpEqual, Val: strings.ToUpper(data.alice.ID)}},
	})
	if err != nil || len(posts) != 2 || posts[0].Title != "first" {
		t.Fatalf("Get by an uppercase key returned %+v, %v, want the posts of Alice", posts, err)
	}

	_, err = Get[UPost](ctx, crud, GetOptions{Filters: sqlfilters.Filters{"EditorID": {Op: sqlfilters.OpIn, Val: []string{"bob"}}}})
	fatalIfNotViolations(t, err, map[string]string{"EditorID": ViolationType})

	upperAliceID := strings.ToUpper(data.alice.ID)
	err = UpdateMultiple[UPost](ctx, crud, map[string]interface{}{"EditorID": &upperAliceID}, UpdateMultipleOptions{
		Filters: sqlfilters.Filters{"Title": {Op: sqlfilters.OpEqual, Val: "third"}},
	})
	if err != nil {
		t.Fatalf("UpdateMultiple failed: %s", err.Error())
	}
	edited, err := Get[UPost](ctx, crud, GetOptions{Filters: sqlfilters.Filters{"EditorID": {Op: sqlfilters.OpEqual, Val: data.alice.ID}}})
	if err != nil || len(edited) != 1 || edited[0].Title != "third" {
		t.Fatalf("Get by the updated key returned %+v, %v, want the third post", edited, err)
	}
}

func testUUIDRelationsOnDelete(t *testing.T, crud *CRUD) {
	data := createUUIDRelationTables(t, crud)
	ctx := context.Background()

	if err := crud.Delete(ctx, data.bob, DeleteOptions{}); err != nil {
		t.Fatalf("Delete failed: %s", err.Error())
	}

	posts, err := Get[UPost](ctx, crud, GetOptions{Order: []string{"Title", "asc"}})
	if err != nil || len(posts) != 2 || posts[0].EditorID != nil {
		t.Fatalf("Get returned %+v, %v, want the posts of Bob deleted, and Bob removed as an editor", posts, err)
	}

	if err := crud.Delete(ctx, data.alice, DeleteOptions{}); err != nil {
		t.Fatalf("Delete failed: %s", err.Error())
	}
	if cnt, err := GetCount[UComment](ctx, crud, GetCountOptions{}); err != nil || cnt != 0 {
		t.Fatalf("GetCount returned %d, %v, want the comments deleted with the posts", cnt, err)
	}
}

func TestUUIDRelations_Preload(t *testing.T) { testUUIDRelationsPreload(t, testCRUD) }
func TestSQLite_UUIDRelations_Preload(t *testing.T) {
	testUUIDRelationsPreload(t, newSQLiteForeignKeysCRUD(t))
}

func TestUUIDRelations_Keys(t *testing.T) { testUUIDRelationsKeys(t, testCRUD) }
func TestSQLite_UUIDRelations_Keys(t *testing.T) {
	testUUIDRelationsKeys(t, newSQLiteForeignKeysCRUD(t))
}

func TestUUIDRelations_OnDelete(t *testing.T) { testUUIDRelationsOnDelete(t, testCRUD) }
func TestSQLite_UUIDRelations_OnDelete(t *testing.T) {
	testUUIDRelationsOnDelete(t, newSQLiteForeignKeysCRUD(t))
}
