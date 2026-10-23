package structinfo

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

type tagsTestStruct struct {
	ID              uint64
	Email           string `crud:"uniq idx"`
	Password        string `crud:"pass"`
	FirstName       string `crud:"idx:full_name"`
	LastName        string `crud:"idx:full_name"`
	Code            string `crud:"uidx"`
	Slug            string `crud:"uidx:slug_idx"`
	Bio             string `crud:"type:text"`
	Title           string `crud:"type:varchar(100)"`
	Renamed         string `crud:"name:renamed_col"`
	InvalidSettings string `crud:"type:blob name:Invalid-Name idx:Invalid-Name"`
	Skipped         string `crud:"-"`
	Status          int
	Tags            []string
	internal        string //nolint:unused // an unexported field, which is not a column
	CreatedAt       time.Time
	CreatedBy       uint64
	ModifiedAt      time.Time
	ModifiedBy      uint64
}

func TestNew_FieldsFromTags(t *testing.T) {
	info := mustNew(t, &tagsTestStruct{}, "crud")

	tests := map[string]FieldInfo{
		"ID":              {},
		"Email":           {Unique: true, Index: true},
		"Password":        {Password: true},
		"FirstName":       {Index: true, IndexName: "full_name"},
		"LastName":        {Index: true, IndexName: "full_name"},
		"Code":            {Index: true, IndexUnique: true},
		"Slug":            {Index: true, IndexUnique: true, IndexName: "slug_idx"},
		"Bio":             {OverrideColumnType: "TEXT"},
		"Title":           {OverrideColumnType: "VARCHAR(100)"},
		"Renamed":         {OverrideColumnName: "renamed_col"},
		"InvalidSettings": {Index: true},
		"Skipped":         {Ignored: true},
		"Status":          {},
	}

	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := info.Fields[name]
			if !ok {
				t.Fatalf("New did not return field %s", name)
			}
			if *got != want {
				t.Fatalf("New returned %+v for field %s, want %+v", *got, name, want)
			}
		})
	}
}

func TestNew_StructInfo(t *testing.T) {
	info := mustNew(t, &tagsTestStruct{}, "crud")

	if _, ok := info.Fields["Tags"]; ok {
		t.Fatal("New returned a field of an unsupported kind")
	}

	wantFieldNames := []string{"ID", "Email", "Password", "FirstName", "LastName", "Code", "Slug", "Bio", "Title",
		"Renamed", "InvalidSettings", "Skipped", "Status", "CreatedAt", "CreatedBy", "ModifiedAt", "ModifiedBy"}
	if !reflect.DeepEqual(info.FieldNames, wantFieldNames) {
		t.Fatalf("New returned field names %v, want %v", info.FieldNames, wantFieldNames)
	}
	wantColumnFieldNames := []string{"ID", "Email", "Password", "FirstName", "LastName", "Code", "Slug", "Bio", "Title",
		"Renamed", "InvalidSettings", "Status", "CreatedAt", "CreatedBy", "ModifiedAt", "ModifiedBy"}
	if !reflect.DeepEqual(info.ColumnFieldNames, wantColumnFieldNames) {
		t.Fatalf("New returned column field names %v, want %v", info.ColumnFieldNames, wantColumnFieldNames)
	}

	wantUpdateFieldNames := []string{"Email", "Password", "FirstName", "LastName", "Code", "Slug", "Bio", "Title",
		"Renamed", "InvalidSettings", "Status", "ModifiedAt", "ModifiedBy"}
	if !reflect.DeepEqual(info.UpdateFieldNames, wantUpdateFieldNames) {
		t.Fatalf("New returned update field names %v, want %v", info.UpdateFieldNames, wantUpdateFieldNames)
	}
	if !reflect.DeepEqual(info.TimeFields, []string{"CreatedAt", "ModifiedAt"}) {
		t.Fatalf("New returned time fields %v, want %v", info.TimeFields, []string{"CreatedAt", "ModifiedAt"})
	}
	if !reflect.DeepEqual(info.UniqueFields, []string{"Email"}) {
		t.Fatalf("New returned unique fields %v, want %v", info.UniqueFields, []string{"Email"})
	}
	if !reflect.DeepEqual(info.PasswordFields, []string{"Password"}) {
		t.Fatalf("New returned password fields %v, want %v", info.PasswordFields, []string{"Password"})
	}
	if !info.ModificationFields {
		t.Fatal("New did not detect modification fields")
	}

	wantIndexes := []*IndexInfo{
		{Fields: []string{"Email"}},
		{Name: "full_name", Fields: []string{"FirstName", "LastName"}},
		{Fields: []string{"Code"}, Unique: true},
		{Name: "slug_idx", Fields: []string{"Slug"}, Unique: true},
		{Fields: []string{"InvalidSettings"}},
	}
	if !reflect.DeepEqual(info.Indexes, wantIndexes) {
		t.Fatalf("New returned indexes %+v, want %+v", info.Indexes, wantIndexes)
	}
}

func TestNew_CustomTagName(t *testing.T) {
	type customTagStruct struct {
		ID    uint64
		Email string `db:"uniq" crud:"pass"`
	}

	info := mustNew(t, &customTagStruct{}, "db")

	if *info.Fields["Email"] != (FieldInfo{Unique: true}) {
		t.Fatalf("New returned %+v for field Email, want only Unique set", *info.Fields["Email"])
	}
}

func TestNew_WithoutModificationFields(t *testing.T) {
	type partialModificationStruct struct {
		ID        uint64
		CreatedAt time.Time
		CreatedBy uint64
		// wrong type, so it does not count as a modification field
		ModifiedAt string
		ModifiedBy uint64
	}

	if mustNew(t, &partialModificationStruct{}, "crud").ModificationFields {
		t.Fatal("New detected modification fields in a struct that does not have all of them")
	}
}

func TestNew_AliasedColumnNames(t *testing.T) {
	type aliasedStruct struct {
		ID       uint64 `crud:"name:t1.id"`
		Name     string `crud:"name:t2.name idx:t2.name_idx"`
		Invalid  string `crud:"name:t1.t2.invalid"`
		Invalid2 string `crud:"name:.invalid"`
	}

	info := mustNew(t, &aliasedStruct{}, "crud")

	if !info.AliasedColumnNames {
		t.Fatal("New did not detect aliased column names")
	}

	want := map[string]FieldInfo{
		"ID":       {OverrideColumnName: "t1.id"},
		"Name":     {OverrideColumnName: "t2.name", Index: true},
		"Invalid":  {},
		"Invalid2": {},
	}
	for name, wantField := range want {
		if *info.Fields[name] != wantField {
			t.Fatalf("New returned %+v for field %s, want %+v", *info.Fields[name], name, wantField)
		}
	}

	if mustNew(t, &tagsTestStruct{}, "crud").AliasedColumnNames {
		t.Fatal("New detected aliased column names in a struct without them")
	}
}

func mustNew(t *testing.T, obj interface{}, tagName string) *StructInfo {
	t.Helper()

	info, err := New(obj, tagName)
	if err != nil {
		t.Fatalf("New failed: %s", err.Error())
	}

	return info
}

func TestNew_InvalidStruct(t *testing.T) {
	type idOnly struct {
		ID uint64
	}
	type idNotFirst struct {
		Name string
		ID   uint64
	}
	type idOfWrongType struct {
		ID   int64
		Name string
	}
	type withoutID struct {
		Name string
	}
	type onlyIgnoredFields struct {
		ID   uint64
		Name string `crud:"-"`
	}
	var nilPointer *idOnly

	tests := map[string]struct {
		obj  interface{}
		want error
	}{
		"nil":                 {nil, ErrNotStructPointer},
		"nil pointer":         {nilPointer, ErrNotStructPointer},
		"struct value":        {idOnly{}, ErrNotStructPointer},
		"pointer to a string": {new(string), ErrNotStructPointer},
		"ID not first":        {&idNotFirst{}, ErrIDField},
		"ID of wrong type":    {&idOfWrongType{}, ErrIDField},
		"without ID":          {&withoutID{}, ErrIDField},
		"ID only":             {&idOnly{}, ErrNoColumns},
		"only ignored fields": {&onlyIgnoredFields{}, ErrNoColumns},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			info, err := New(test.obj, "crud")
			if !errors.Is(err, test.want) {
				t.Fatalf("New returned err %v, want %v", err, test.want)
			}
			if info != nil {
				t.Fatal("New returned struct info along with an err")
			}
		})
	}
}

func TestIsFieldTypeSupported(t *testing.T) {
	type status string
	type nested struct{ Name string }

	tests := map[reflect.Type]bool{
		reflect.TypeOf(""):             true,
		reflect.TypeOf(int8(0)):        true,
		reflect.TypeOf(uint64(0)):      true,
		reflect.TypeOf(0.5):            true,
		reflect.TypeOf(true):           true,
		reflect.TypeOf(status("")):     true,
		reflect.TypeOf(time.Time{}):    true,
		reflect.TypeOf(&time.Time{}):   true,
		reflect.TypeOf(new(string)):    true,
		reflect.TypeOf(new(*string)):   false,
		reflect.TypeOf(&[]string{}):    false,
		reflect.TypeOf(&nested{}):      false,
		reflect.TypeOf(time.Second):    true,
		reflect.TypeOf(nested{}):       false,
		reflect.TypeOf([]string{}):     false,
		reflect.TypeOf(map[int]bool{}): false,
	}
	for typ, want := range tests {
		if got := IsFieldTypeSupported(typ); got != want {
			t.Fatalf("IsFieldTypeSupported(%v) returned %v, want %v", typ, got, want)
		}
	}
}

func TestNew_UpdateFieldNamesWithoutModificationFields(t *testing.T) {
	type withoutModificationFields struct {
		ID        uint64
		Name      string
		CreatedAt int64
		CreatedBy uint64
	}

	info := mustNew(t, &withoutModificationFields{}, "crud")
	want := []string{"Name", "CreatedAt", "CreatedBy"}
	if info.ModificationFields || !reflect.DeepEqual(info.UpdateFieldNames, want) {
		t.Fatalf("New returned update field names %v, want %v", info.UpdateFieldNames, want)
	}
}

func TestNew_NullableFields(t *testing.T) {
	type nullableStruct struct {
		ID        uint64
		Name      string
		Nickname  *string
		BirthDate *time.Time
	}

	info := mustNew(t, &nullableStruct{}, "crud")
	if info.Fields["Name"].Nullable || !info.Fields["Nickname"].Nullable || !info.Fields["BirthDate"].Nullable {
		t.Fatalf("New returned wrong nullable fields: %+v %+v %+v", *info.Fields["Name"], *info.Fields["Nickname"], *info.Fields["BirthDate"])
	}
	if !reflect.DeepEqual(info.TimeFields, []string{"BirthDate"}) {
		t.Fatalf("New returned time fields %v, want [BirthDate]", info.TimeFields)
	}
}

func TestNew_PasswordFieldOfWrongType(t *testing.T) {
	for _, obj := range []interface{}{
		&struct {
			ID       uint64
			Password *string `crud:"pass"`
		}{},
		&struct {
			ID  uint64
			PIN int `crud:"pass"`
		}{},
	} {
		if _, err := New(obj, "crud"); !errors.Is(err, ErrPasswordField) {
			t.Fatalf("New returned err %v for %T, want %v", err, obj, ErrPasswordField)
		}
	}
}
