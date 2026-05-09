package cli

import (
	"path/filepath"
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantSkillsDir func(cwd string) string
		wantJPDir     func(cwd string) string
		wantRemaining []string
	}{
		{
			name:          "両フラグ指定でフラグ値が採用される",
			args:          []string{"--skills-dir", "/abs/skills", "--skills-jp-dir", "/abs/skills-jp", "status", "foo"},
			wantSkillsDir: func(_ string) string { return "/abs/skills" },
			wantJPDir:     func(_ string) string { return "/abs/skills-jp" },
			wantRemaining: []string{"status", "foo"},
		},
		{
			name:          "フラグなしで cwd デフォルトにフォールバック",
			args:          []string{"status", "foo"},
			wantSkillsDir: func(cwd string) string { return filepath.Join(cwd, "skills") },
			wantJPDir:     func(cwd string) string { return filepath.Join(cwd, "skills-jp") },
			wantRemaining: []string{"status", "foo"},
		},
		{
			name:          "skills-dir のみ指定で skills-jp-dir は cwd デフォルト",
			args:          []string{"--skills-dir", "/abs/skills", "record", "bar"},
			wantSkillsDir: func(_ string) string { return "/abs/skills" },
			wantJPDir:     func(cwd string) string { return filepath.Join(cwd, "skills-jp") },
			wantRemaining: []string{"record", "bar"},
		},
		{
			name:          "相対パスは cwd 起点で絶対パスに正規化される",
			args:          []string{"--skills-dir", "rel/skills", "status", "foo"},
			wantSkillsDir: func(cwd string) string { return filepath.Join(cwd, "rel/skills") },
			wantJPDir:     func(cwd string) string { return filepath.Join(cwd, "skills-jp") },
			wantRemaining: []string{"status", "foo"},
		},
		{
			name:          "サブコマンドなしでも remaining は空で返る",
			args:          []string{"--skills-dir", "/abs/skills"},
			wantSkillsDir: func(_ string) string { return "/abs/skills" },
			wantJPDir:     func(cwd string) string { return filepath.Join(cwd, "skills-jp") },
			wantRemaining: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cwd := t.TempDir()
			t.Chdir(cwd)

			cfg, rest, err := Resolve(tt.args)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}

			// Compare via EvalSymlinks to absorb the macOS /var -> /private/var symlink difference.
			gotSkills := evalOrSelf(t, cfg.SkillsDir)
			wantSkills := evalOrSelf(t, tt.wantSkillsDir(cwd))
			if gotSkills != wantSkills {
				t.Errorf("SkillsDir = %q, want %q", gotSkills, wantSkills)
			}

			gotJP := evalOrSelf(t, cfg.SkillsJPDir)
			wantJP := evalOrSelf(t, tt.wantJPDir(cwd))
			if gotJP != wantJP {
				t.Errorf("SkillsJPDir = %q, want %q", gotJP, wantJP)
			}

			if !slicesEqual(rest, tt.wantRemaining) {
				t.Errorf("remaining = %v, want %v", rest, tt.wantRemaining)
			}
		})
	}
}

func TestResolveRejectsUnknownFlag(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, _, err := Resolve([]string{"--unknown", "status", "foo"}); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func evalOrSelf(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.EvalSymlinks(p)
	if err != nil {
		// Path may not exist yet (e.g. only the flag value was given); return it as-is.
		return p
	}
	return abs
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
