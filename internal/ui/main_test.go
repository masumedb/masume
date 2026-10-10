package ui

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	insideTmux = false
	os.Exit(m.Run())
}
