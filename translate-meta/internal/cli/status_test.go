package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func createTempSkill(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		fullPath := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestHashDir(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name      string
		files1    map[string]string
		files2    map[string]string
		wantEqual bool
		wantErr   bool
	}{
		{
			name:      "同一内容で同一ハッシュ",
			files1:    map[string]string{"a.md": "hello", "b.md": "world"},
			files2:    map[string]string{"a.md": "hello", "b.md": "world"},
			wantEqual: true,
		},
		{
			name:      "内容が異なればハッシュも異なる",
			files1:    map[string]string{"a.md": "hello"},
			files2:    map[string]string{"a.md": "changed"},
			wantEqual: false,
		},
		{
			name:      "ファイル名が異なればハッシュも異なる",
			files1:    map[string]string{"a.md": "hello"},
			files2:    map[string]string{"b.md": "hello"},
			wantEqual: false,
		},
		{
			name:      "サブディレクトリ内のファイルも含まれる",
			files1:    map[string]string{"sub/a.md": "hello"},
			files2:    map[string]string{"sub/a.md": "hello"},
			wantEqual: true,
		},
		{
			name:      "ファイル順序非依存",
			files1:    map[string]string{"z.md": "first", "a.md": "second"},
			files2:    map[string]string{"a.md": "second", "z.md": "first"},
			wantEqual: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir1 := createTempSkill(t, tt.files1)
			dir2 := createTempSkill(t, tt.files2)

			h1, err := hashDir(ctx, dir1)
			if err != nil {
				t.Fatalf("hashDir(dir1): %v", err)
			}
			h2, err := hashDir(ctx, dir2)
			if err != nil {
				t.Fatalf("hashDir(dir2): %v", err)
			}

			if tt.wantEqual && h1 != h2 {
				t.Errorf("expected equal hashes, got %s != %s", h1, h2)
			}
			if !tt.wantEqual && h1 == h2 {
				t.Errorf("expected different hashes, got same: %s", h1)
			}
		})
	}

	t.Run("空ディレクトリ", func(t *testing.T) {
		dir := t.TempDir()
		hash, err := hashDir(ctx, dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hash == "" {
			t.Error("expected non-empty hash for empty dir")
		}
	})

	t.Run("存在しないディレクトリ", func(t *testing.T) {
		_, err := hashDir(ctx, "/nonexistent-dir-that-does-not-exist")
		if err == nil {
			t.Fatal("expected error for nonexistent dir")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("expected fs.ErrNotExist, got: %v", err)
		}
	})
}

func TestRunStatus(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{
			name:     "引数なしでERROR",
			args:     []string{},
			wantCode: ERROR,
		},
		{
			name:     "引数過多でERROR",
			args:     []string{"a", "b"},
			wantCode: ERROR,
		},
		{
			name:     "存在しないディレクトリでERROR",
			args:     []string{"/nonexistent"},
			wantCode: ERROR,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := RunStatus(ctx, tt.args)
			if code != tt.wantCode {
				t.Errorf("RunStatus(%v) = %d, want %d", tt.args, code, tt.wantCode)
			}
		})
	}

	t.Run("有効なディレクトリでUP_TO_DATE", func(t *testing.T) {
		dir := createTempSkill(t, map[string]string{"readme.md": "content"})
		code := RunStatus(ctx, []string{dir})
		if code != UP_TO_DATE {
			t.Errorf("RunStatus = %d, want %d (UP_TO_DATE)", code, UP_TO_DATE)
		}
	})
}
