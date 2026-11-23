package migrate

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestDiscoverSorted(t *testing.T) {
	fsys := fstest.MapFS{
		"0003_c.sql": {Data: []byte("select 3;")},
		"0001_a.sql": {Data: []byte("select 1;")},
		"0002_b.sql": {Data: []byte("select 2;")},
	}
	files, err := Discover(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 || files[0].Version != 1 || files[1].Version != 2 || files[2].Version != 3 {
		t.Fatalf("expect ascending versions, got %+v", files)
	}
	if files[0].Name != "a" || files[0].Checksum == "" {
		t.Fatalf("expect parsed name and checksum, got %+v", files[0])
	}
}

func TestDiscoverDuplicateVersion(t *testing.T) {
	fsys := fstest.MapFS{
		"0001_a.sql": {Data: []byte("select 1;")},
		"0001_b.sql": {Data: []byte("select 1;")},
	}
	if _, err := Discover(fsys); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expect duplicate error, got %v", err)
	}
}

func TestDiscoverInvalidName(t *testing.T) {
	fsys := fstest.MapFS{"1_a.sql": {Data: []byte("select 1;")}}
	if _, err := Discover(fsys); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expect invalid name error, got %v", err)
	}
}

func TestDiscoverEmptyFile(t *testing.T) {
	fsys := fstest.MapFS{"0001_empty.sql": {Data: []byte("   ")}}
	if _, err := Discover(fsys); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expect empty error, got %v", err)
	}
}

func TestDiscoverIgnoresNonSQL(t *testing.T) {
	fsys := fstest.MapFS{
		"README.md":      {Data: []byte("docs")},
		"0001_a.sql":     {Data: []byte("select 1;")},
		"0001_a.sql.bak": {Data: []byte("junk")},
	}
	files, err := Discover(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expect only .sql discovered, got %+v", files)
	}
}

func TestCompareChecksumConflict(t *testing.T) {
	files := []Migration{{Version: 1, Checksum: "y"}}
	conflicts := Compare(map[int64]string{1: "x"}, files)
	if len(conflicts) != 1 || conflicts[0].Version != 1 || conflicts[0].Applied != "x" || conflicts[0].Expected != "y" {
		t.Fatalf("expect checksum conflict, got %+v", conflicts)
	}
}

func TestCompareMissingFile(t *testing.T) {
	conflicts := Compare(map[int64]string{2: "y"}, []Migration{{Version: 1, Checksum: "x"}})
	if len(conflicts) != 1 || conflicts[0].Version != 2 || conflicts[0].Expected != "" {
		t.Fatalf("expect missing-file conflict, got %+v", conflicts)
	}
}

func TestExpectedVersionEmpty(t *testing.T) {
	if ExpectedVersion(nil) != 0 {
		t.Fatal("empty list expected 0")
	}
}