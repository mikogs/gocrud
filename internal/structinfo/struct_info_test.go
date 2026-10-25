package structinfo

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

// money is written as text, e.g. "0.00", as decimal types are
type money struct{}

func (m money) Value() (driver.Value, error) { return "0.00", nil }
func (m *money) Scan(any) error              { return nil }

// nullValuer is written as NULL when it is zero, and is not a struct of a value and Valid
type nullValuer struct{}

func (n nullValuer) Value() (driver.Value, error) { return nil, nil }
func (n *nullValuer) Scan(any) error              { return nil }

type panickingValuer struct{ value *string }

func (p panickingValuer) Value() (driver.Value, error) { return *p.value, nil }
func (p *panickingValuer) Scan(any) error              { return nil }

type pointerValuer struct{}

func (*pointerValuer) Value() (driver.Value, error) { return int64(0), nil }
func (*pointerValuer) Scan(any) error               { return nil }

// valuerOnly cannot be read, as it is not a sql.Scanner
type valuerOnly struct{}

func (valuerOnly) Value() (driver.Value, error) { return "", nil }

// panickingJSON panics in MarshalJSON of its zero value
type panickingJSON struct{ value *string }

func (p panickingJSON) MarshalJSON() ([]byte, error) { return []byte(*p.value), nil }

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
	InvalidSettings string `crud:"name:Invalid-Name idx:Invalid-Name"`
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

func mustNew(t *testing.T, obj any, tagName string) *StructInfo {
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
	type invalidColumnType struct {
		ID   uint64
		Name string `crud:"type:varchar(20);drop"`
	}
	type valuerNullWithoutType struct {
		ID    uint64
		Value nullValuer
	}
	type valuerPanicking struct {
		ID    uint64
		Value panickingValuer
	}
	type jsonID struct {
		ID   uint64 `crud:"json"`
		Name string
	}
	type jsonPassword struct {
		ID       uint64
		Password string `crud:"pass json"`
	}
	type jsonValuer struct {
		ID    uint64
		Price money `crud:"json"`
	}
	type jsonFunc struct {
		ID       uint64
		Callback func() `crud:"json"`
	}
	type jsonPanicking struct {
		ID    uint64
		Value panickingJSON `crud:"json"`
	}
	type jsonInvalidColumnType struct {
		ID       uint64
		Settings map[string]int `crud:"json type:jsonb;drop"`
	}
	type jsonRelation struct {
		ID         uint64
		CustomerID uint64
		Customer   *idOnly `crud:"rel:CustomerID json"`
	}
	var nilPointer *idOnly

	tests := map[string]struct {
		obj  any
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
		"invalid type tag":    {&invalidColumnType{}, ErrColumnType},
		"NULL zero value":     {&valuerNullWithoutType{}, ErrColumnType},
		"panicking Value":     {&valuerPanicking{}, ErrColumnType},
		"JSON ID":             {&jsonID{}, ErrColumnType},
		"JSON password":       {&jsonPassword{}, ErrColumnType},
		"JSON Valuer":         {&jsonValuer{}, ErrColumnType},
		"JSON of a func":      {&jsonFunc{}, ErrColumnType},
		"panicking JSON":      {&jsonPanicking{}, ErrColumnType},
		"JSON type tag":       {&jsonInvalidColumnType{}, ErrColumnType},
		"JSON relation":       {&jsonRelation{}, ErrRelation},
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
		reflect.TypeOf(""):                true,
		reflect.TypeOf(int8(0)):           true,
		reflect.TypeOf(uint64(0)):         true,
		reflect.TypeOf(0.5):               true,
		reflect.TypeOf(true):              true,
		reflect.TypeOf(status("")):        true,
		reflect.TypeOf(time.Time{}):       true,
		reflect.TypeOf(&time.Time{}):      true,
		reflect.TypeOf(new(string)):       true,
		reflect.TypeOf(new(*string)):      false,
		reflect.TypeOf(&[]string{}):       false,
		reflect.TypeOf(&nested{}):         false,
		reflect.TypeOf(time.Second):       true,
		reflect.TypeOf(nested{}):          false,
		reflect.TypeOf([]string{}):        false,
		reflect.TypeOf(map[int]bool{}):    false,
		reflect.TypeOf([]byte{}):          true,
		reflect.TypeOf(&[]byte{}):         true,
		reflect.TypeOf(json.RawMessage{}): true,
		reflect.TypeOf(sql.NullString{}):  true,
		reflect.TypeOf(&money{}):          true,
		reflect.TypeOf(uuid.UUID{}):       true,
		reflect.TypeOf(pointerValuer{}):   true,
		reflect.TypeOf(valuerOnly{}):      false,
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
	for _, obj := range []any{
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

func TestNew_ValuerFields(t *testing.T) {
	type valuers struct {
		ID       uint64
		Price    money
		Discount *money
		Note     sql.NullString
		Value    nullValuer `crud:"type:text"`
	}

	info := mustNew(t, &valuers{}, "crud")
	for name, nullable := range map[string]bool{"Price": false, "Discount": true, "Note": true, "Value": true} {
		if info.Fields[name].Nullable != nullable {
			t.Fatalf("New returned Nullable %v for %s, want %v", info.Fields[name].Nullable, name, nullable)
		}
	}
}

func TestNew_JSONFields(t *testing.T) {
	type address struct{ City string }
	type withJSON struct {
		ID       uint64
		Settings map[string]int `crud:"json"`
		Address  *address       `crud:"json"`
		At       time.Time      `crud:"json"`
		Ignored  []string       `crud:"-"`
		Tags     []string
		internal []string `crud:"json"` //nolint:unused // an unexported field, which is not stored
	}

	info := mustNew(t, &withJSON{}, "crud")
	if !reflect.DeepEqual(info.JSONFields, []string{"Settings", "Address", "At"}) || !reflect.DeepEqual(info.FieldNames, []string{"ID", "Settings", "Address", "At"}) {
		t.Fatalf("New returned JSONFields %v and FieldNames %v", info.JSONFields, info.FieldNames)
	}
	if len(info.TimeFields) != 0 {
		t.Fatalf("New returned TimeFields %v, want none, as At is stored as JSON", info.TimeFields)
	}
	for name, nullable := range map[string]bool{"Settings": false, "Address": true, "At": false} {
		if !info.Fields[name].JSON || info.Fields[name].Nullable != nullable {
			t.Fatalf("New returned %+v for %s, want JSON and Nullable %v", info.Fields[name], name, nullable)
		}
	}
}

func TestColumnGoType(t *testing.T) {
	type level int

	tests := map[reflect.Type]string{
		reflect.TypeOf(level(0)):          "int",
		reflect.TypeOf(new(level)):        "int",
		reflect.TypeOf(time.Time{}):       "time.Time",
		reflect.TypeOf([]byte{}):          "[]byte",
		reflect.TypeOf(json.RawMessage{}): "[]byte",
		reflect.TypeOf(money{}):           "string",
		reflect.TypeOf(&money{}):          "string",
		reflect.TypeOf(uuid.UUID{}):       "string",
		reflect.TypeOf(sql.NullString{}):  "string",
		reflect.TypeOf(sql.NullInt64{}):   "int64",
		reflect.TypeOf(sql.NullTime{}):    "time.Time",
		reflect.TypeOf(sql.Null[bool]{}):  "bool",
		reflect.TypeOf(uuid.NullUUID{}):   "string",
		reflect.TypeOf(pointerValuer{}):   "int64",
		reflect.TypeOf(nullValuer{}):      "",
		reflect.TypeOf([]string{}):        "",
	}
	for typ, want := range tests {
		if got := ColumnGoType(typ); got != want {
			t.Fatalf("ColumnGoType(%v) returned %q, want %q", typ, got, want)
		}
	}
}

// int64Only scans only int64, as the values of drivers are
type int64Only struct{ value int64 }

func (i int64Only) Value() (driver.Value, error) { return i.value, nil }
func (i *int64Only) Scan(src any) error {
	value, ok := src.(int64)
	if !ok {
		return errors.New("not an int64")
	}
	i.value = value
	return nil
}

func TestScanValue(t *testing.T) {
	if value, ok := ScanValue(reflect.TypeOf(int64Only{}), 5); !ok || value.Interface() != (int64Only{value: 5}) {
		t.Fatalf("ScanValue returned %v, %v, want 5 from an int", value, ok)
	}
	if value, ok := ScanValue(reflect.TypeOf(sql.NullInt64{}), uint8(5)); !ok || value.Interface() != (sql.NullInt64{Int64: 5, Valid: true}) {
		t.Fatalf("ScanValue returned %v, %v, want 5", value, ok)
	}
	if _, ok := ScanValue(reflect.TypeOf(sql.NullInt64{}), "x"); ok {
		t.Fatal("ScanValue scanned x into sql.NullInt64")
	}
	if _, ok := ScanValue(reflect.TypeOf(sql.NullInt64{}), struct{}{}); ok {
		t.Fatal("ScanValue scanned a struct, which is not a value of a driver")
	}
}

func TestZeroValue(t *testing.T) {
	if value, err := ZeroValue(reflect.TypeOf(money{})); value != "0.00" || err != nil {
		t.Fatalf("ZeroValue returned %v, %v, want 0.00", value, err)
	}
	if _, err := ZeroValue(reflect.TypeOf(panickingValuer{})); err == nil {
		t.Fatal("ZeroValue did not return an err for a Value method that panics")
	}
}
