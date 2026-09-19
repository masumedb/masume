package notebook_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/turanmahmudov/masume/internal/notebook"
)

// The notebook file is written whole. A write that stops in the middle would otherwise
// leave the file it replaced truncated.
func TestSaveWritesTheFileWhole(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "book.md")

	if err := notebook.Save(path, notebook.Parse("```sql id=one\nselect 1\n```\n")); err != nil {
		t.Fatalf("the first save failed: %v", err)
	}
	if err := notebook.Save(path, notebook.Parse("```sql id=two\nselect 2\n```\n")); err != nil {
		t.Fatalf("the second save failed: %v", err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		names := []string{}
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Errorf("the directory holds %v, wanted the notebook alone", names)
	}
	held, err := notebook.Read(path)
	if err != nil {
		t.Fatalf("the notebook did not read back: %v", err)
	}
	if len(held.Cells) != 1 || held.Cells[0].ID != "two" {
		t.Errorf("the file holds %d cells, wanted the second save", len(held.Cells))
	}
}
