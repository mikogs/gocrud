package gocrud

import (
	"reflect"
	"testing"
)

func TestRelatedType(t *testing.T) {
	tests := []struct {
		obj   interface{}
		field string
		want  reflect.Type
	}{
		{&PFPost{}, "Author", reflect.TypeOf(PFUser{})},
		{&UAuthor{}, "Posts", reflect.TypeOf(UPost{})},
		{&PFPost{}, "Title", nil},
		{&PFPost{}, "Unknown", nil},
	}
	for _, test := range tests {
		got, ok := testCRUD.RelatedType(test.obj, test.field)
		if got != test.want || ok != (test.want != nil) {
			t.Fatalf("RelatedType of %T.%s returned %v, %v, want %v", test.obj, test.field, got, ok, test.want)
		}
	}
}
