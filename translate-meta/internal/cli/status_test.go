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

// makeSkillFixture creates skills/<name> and skills-jp/<name> under cfg, placing
// the given files only on the skills-jp side. The skills side is left empty.
func makeSkillFixture(t *testing.T, skillName string, files map[string]string) Config {
	t.Helper()
	root := t.TempDir()
	cfg := Config{
		SkillsDir:   filepath.Join(root, "skills"),
		SkillsJPDir: filepath.Join(root, "skills-jp"),
	}
	if err := os.MkdirAll(filepath.Join(cfg.SkillsDir, skillName), 0o755); err != nil {
		t.Fatal(err)
	}
	jpDir := filepath.Join(cfg.SkillsJPDir, skillName)
	if err := os.MkdirAll(jpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		fullPath := filepath.Join(jpDir, name)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return cfg
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

	t.Run("引数なしでERROR", func(t *testing.T) {
		cfg := Config{SkillsDir: t.TempDir(), SkillsJPDir: t.TempDir()}
		if got := RunStatus(ctx, cfg, []string{}); got != ERROR {
			t.Errorf("RunStatus = %d, want %d", got, ERROR)
		}
	})

	t.Run("引数過多でERROR", func(t *testing.T) {
		cfg := Config{SkillsDir: t.TempDir(), SkillsJPDir: t.TempDir()}
		if got := RunStatus(ctx, cfg, []string{"a", "b"}); got != ERROR {
			t.Errorf("RunStatus = %d, want %d", got, ERROR)
		}
	})

	t.Run(".translate-meta が無いとUNTRANSLATED", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		if got := RunStatus(ctx, cfg, []string{"foo"}); got != UNTRANSLATED {
			t.Errorf("RunStatus = %d, want %d (UNTRANSLATED)", got, UNTRANSLATED)
		}
	})

	t.Run("skills-jp が無いとERROR", func(t *testing.T) {
		// skills/<name>/.translate-meta exists but skills-jp/<name> does not.
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		writeMeta(t, cfg, "foo", "deadbeef")
		if err := os.RemoveAll(filepath.Join(cfg.SkillsJPDir, "foo")); err != nil {
			t.Fatal(err)
		}
		if got := RunStatus(ctx, cfg, []string{"foo"}); got != ERROR {
			t.Errorf("RunStatus = %d, want %d (ERROR)", got, ERROR)
		}
	})

	t.Run("ハッシュ一致でUP_TO_DATE", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		hash, err := hashDir(ctx, filepath.Join(cfg.SkillsJPDir, "foo"))
		if err != nil {
			t.Fatal(err)
		}
		writeMeta(t, cfg, "foo", hash)

		if got := RunStatus(ctx, cfg, []string{"foo"}); got != UP_TO_DATE {
			t.Errorf("RunStatus = %d, want %d (UP_TO_DATE)", got, UP_TO_DATE)
		}
	})

	t.Run("ハッシュ不一致でNEEDS_UPDATE", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		writeMeta(t, cfg, "foo", "stale-hash")

		if got := RunStatus(ctx, cfg, []string{"foo"}); got != NEEDS_UPDATE {
			t.Errorf("RunStatus = %d, want %d (NEEDS_UPDATE)", got, NEEDS_UPDATE)
		}
	})

	t.Run("空メタファイルでNEEDS_UPDATE", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		writeMeta(t, cfg, "foo", "")

		if got := RunStatus(ctx, cfg, []string{"foo"}); got != NEEDS_UPDATE {
			t.Errorf("RunStatus = %d, want %d (NEEDS_UPDATE)", got, NEEDS_UPDATE)
		}
	})
}

func writeMeta(t *testing.T, cfg Config, skillName, content string) {
	t.Helper()
	dir := filepath.Join(cfg.SkillsDir, skillName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, metaFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
