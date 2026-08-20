package migrate

import (
	"testing"
	"testing/fstest"
)

func TestLoadSortsMigrations(t *testing.T) {
	t.Parallel()

	items, err := load(fstest.MapFS{
		"000002_second.up.sql": {Data: []byte("SELECT 2;")},
		"000001_first.up.sql":  {Data: []byte("SELECT 1;")},
		"README.md":            {Data: []byte("ignored")},
	})
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("migration count = %d, want 2", len(items))
	}
	if items[0].version != 1 || items[1].version != 2 {
		t.Fatalf("versions = [%d, %d], want [1, 2]", items[0].version, items[1].version)
	}
}

func TestLoadRejectsDuplicateVersion(t *testing.T) {
	t.Parallel()

	_, err := load(fstest.MapFS{
		"000001_first.up.sql": {Data: []byte("SELECT 1;")},
		"1_other.up.sql":      {Data: []byte("SELECT 2;")},
	})
	if err == nil {
		t.Fatal("expected duplicate version error")
	}
}

func TestLoadRejectsEmptySet(t *testing.T) {
	t.Parallel()

	_, err := load(fstest.MapFS{"README.md": {Data: []byte("ignored")}})
	if err == nil {
		t.Fatal("expected empty migration set error")
	}
}
