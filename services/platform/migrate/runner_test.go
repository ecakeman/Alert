package migrate

import (
	"errors"
	"testing"
	"testing/fstest"
)

func TestDiscoverSortsAndParses(t *testing.T) {
	fsys := fstest.MapFS{
		"0002_second.sql": &fstest.MapFile{Data: []byte("SELECT 2;")},
		"0001_first.sql":  &fstest.MapFile{Data: []byte("SELECT 1;")},
		"notes.txt":       &fstest.MapFile{Data: []byte("ignored")},
	}
	files, err := Discover(fsys)
	if err != nil {
		t.Fatalf("Discover 失败: %v", err)
	}
	if len(files) != 2 || files[0].Version != 1 || files[1].Version != 2 {
		t.Fatalf("排序错误: %+v", files)
	}
	if files[0].Checksum == "" || files[0].Name != "first" {
		t.Fatalf("解析错误: %+v", files[0])
	}
}

func TestDiscoverRejectsInvalidFilename(t *testing.T) {
	_, err := Discover(fstest.MapFS{
		"1_bad.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
	})
	if err == nil {
		t.Fatal("非法文件名应报错")
	}
}

func TestValidateRejectsDuplicateVersion(t *testing.T) {
	err := Validate([]Migration{
		{Version: 1, Path: "0001_a.sql", SQL: "SELECT 1;"},
		{Version: 1, Path: "0001_b.sql", SQL: "SELECT 2;"},
	})
	if err == nil {
		t.Fatal("重复版本应报错")
	}
}

func TestCompareDetectsChecksumAndMissing(t *testing.T) {
	files := []Migration{
		{Version: 1, Checksum: "aaa"},
		{Version: 2, Checksum: "bbb"},
	}
	conflicts := Compare(map[int64]string{1: "zzz", 3: "ccc"}, files)
	if len(conflicts) != 2 {
		t.Fatalf("冲突数 = %d，期望 2", len(conflicts))
	}
	if conflicts[0].Version != 1 || conflicts[0].Expected != "aaa" {
		t.Fatalf("冲突 1 = %+v", conflicts[0])
	}
	if conflicts[1].Version != 3 || conflicts[1].Expected != "" {
		t.Fatalf("冲突 2 = %+v", conflicts[1])
	}
}

func TestTargetValidate(t *testing.T) {
	valid := Target{Schema: "monitor", VersionTable: "monitor.schema_migrations", LockKey: "alert.monitor.migrate", FS: fstest.MapFS{}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("合法目标被拒绝: %v", err)
	}

	cases := []struct {
		name   string
		target Target
		field  string
	}{
		{"bad schema", Target{Schema: "bad-name", VersionTable: "bad-name.v", LockKey: "k", FS: fstest.MapFS{}}, "Schema"},
		{"schema mismatch", Target{Schema: "monitor", VersionTable: "registry.v", LockKey: "k", FS: fstest.MapFS{}}, "VersionTable"},
		{"empty lock", Target{Schema: "monitor", VersionTable: "monitor.v", LockKey: "", FS: fstest.MapFS{}}, "LockKey"},
		{"nil fs", Target{Schema: "monitor", VersionTable: "monitor.v", LockKey: "k"}, "FS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.target.Validate()
			var te *TargetError
			if !errors.As(err, &te) || te.Field != tc.field {
				t.Fatalf("期望 %s TargetError，实际 %v", tc.field, err)
			}
		})
	}
}
