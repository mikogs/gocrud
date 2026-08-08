package structinfo

// IndexInfo describes a database index built from one or more struct fields
// tagged with 'idx' or 'uidx'.
type IndexInfo struct {
	// Name is the name given via 'idx:<name>' or 'uidx:<name>'. Empty when the
	// index was declared without a name (a single-field index with an auto-generated name).
	Name string
	// Fields contains the ordered list of struct field names included in the index,
	// in the order they are declared in the struct.
	Fields []string
	// Unique is set to true when the index was declared with 'uidx' (or 'uidx:<name>').
	Unique bool
}

// buildIndexes groups fields tagged with 'idx'/'uidx' into IndexInfo entries.
// Fields sharing the same (non-empty) IndexName are combined into a single composite
// index; unnamed fields each get their own single-column index.
func (s *StructInfo) buildIndexes() {
	groups := make(map[string]*IndexInfo)
	order := make([]string, 0)

	for _, fieldName := range s.FieldNames {
		field := s.Fields[fieldName]
		if !field.Index {
			continue
		}

		key := field.IndexName
		if key == "" {
			// Unnamed indexes are never grouped, so give each one a unique key.
			key = "\x00" + fieldName
		}

		group, ok := groups[key]
		if !ok {
			group = &IndexInfo{Name: field.IndexName}
			groups[key] = group
			order = append(order, key)
		}

		group.Fields = append(group.Fields, fieldName)
		if field.IndexUnique {
			group.Unique = true
		}
	}

	s.Indexes = make([]*IndexInfo, 0, len(order))
	for _, key := range order {
		s.Indexes = append(s.Indexes, groups[key])
	}
}
