package schemadiff

import (
	"strings"
)

var changeMarks = map[Change]string{ChangeAdded: "+", ChangeRemoved: "-", ChangeAltered: "~"}

// WriteReport returns the differences as text: one block per table, one line per item.
func WriteReport(found []Difference) string {
	if len(found) == 0 {
		return "no differences\n"
	}
	var text strings.Builder
	table := ""
	for _, difference := range found {
		if difference.Table != table {
			if table != "" {
				text.WriteString("\n")
			}
			table = difference.Table
			text.WriteString(table + "\n")
		}
		text.WriteString("  " + DescribeDifference(difference) + "\n")
	}
	return text.String()
}

// DescribeDifference returns one difference as a line, such as `~ column total  integer -> bigint`.
// A table line has the relation kind only, such as `+ view`.
func DescribeDifference(difference Difference) string {
	mark := changeMarks[difference.Change]
	if difference.Part == PartTable {
		switch difference.Change {
		case ChangeAdded:
			return mark + " " + difference.Target
		case ChangeRemoved:
			return mark + " " + difference.Source
		}
		return mark + " " + difference.Source + " -> " + difference.Target
	}
	line := mark + " " + string(difference.Part) + " " + difference.Name
	switch difference.Change {
	case ChangeAdded:
		return line + "  " + difference.Target
	case ChangeRemoved:
		return line + "  " + difference.Source
	}
	return line + "  " + difference.Source + " -> " + difference.Target
}
