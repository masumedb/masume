package app

// SchemaCompare is the source schema of the connection of the tab, and the target schema of
// a connection. An empty TargetProfile is the connection of the tab.
type SchemaCompare struct {
	SourceSchema  string
	TargetProfile string
	TargetSchema  string
}

// DescribeTitle returns the two schemas, such as `public → prod.public`.
func (compare SchemaCompare) DescribeTitle() string {
	target := compare.TargetSchema
	if compare.TargetProfile != "" {
		target = compare.TargetProfile + "." + target
	}
	return compare.SourceSchema + " → " + target
}

// NewCompareTab returns a tab that shows the differences between two schemas.
func NewCompareTab(id int, compare SchemaCompare) *Tab {
	tab := newTab(id, TabCompare, "")
	tab.Compare = &compare
	tab.View = ViewDiff
	return tab
}

// OpenCompare opens a compare tab, or shows the one that compares the same schemas.
func (connection *Connection) OpenCompare(compare SchemaCompare) *Tab {
	for at, tab := range connection.Tabs {
		if tab.Kind == TabCompare && tab.Compare != nil && *tab.Compare == compare {
			connection.ActiveIndex = at
			return tab
		}
	}
	connection.nextTabID++
	return connection.showTab(NewCompareTab(connection.nextTabID, compare))
}
