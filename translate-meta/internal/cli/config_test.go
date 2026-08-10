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

func TestResolve_AgentMode(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantAgentsDir func(cwd string) string
		wantJPDir     func(cwd string) string
		wantRemaining []string
	}{
		{
			name:          "両フラグ指定でフラグ値が採用される",
			args:          []string{"--agents-dir", "/abs/agents", "--agents-jp-dir", "/abs/agents-jp", "status", "reviewer"},
			wantAgentsDir: func(_ string) string { return "/abs/agents" },
			wantJPDir:     func(_ string) string { return "/abs/agents-jp" },
			wantRemaining: []string{"status", "reviewer"},
		},
		{
			name:          "agents-dir のみ指定で agents-jp-dir は cwd デフォルト",
			args:          []string{"--agents-dir", "/abs/agents", "record", "reviewer"},
			wantAgentsDir: func(_ string) string { return "/abs/agents" },
			wantJPDir:     func(cwd string) string { return filepath.Join(cwd, "agents-jp") },
			wantRemaining: []string{"record", "reviewer"},
		},
		{
			name:          "agents-jp-dir のみ指定で agents-dir は cwd デフォルト",
			args:          []string{"--agents-jp-dir", "/abs/agents-jp", "record", "reviewer"},
			wantAgentsDir: func(cwd string) string { return filepath.Join(cwd, "agents") },
			wantJPDir:     func(_ string) string { return "/abs/agents-jp" },
			wantRemaining: []string{"record", "reviewer"},
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

			if cfg.Mode != ModeAgents {
				t.Errorf("Mode = %v, want ModeAgents", cfg.Mode)
			}

			gotAgents := evalOrSelf(t, cfg.AgentsDir)
			wantAgents := evalOrSelf(t, tt.wantAgentsDir(cwd))
			if gotAgents != wantAgents {
				t.Errorf("AgentsDir = %q, want %q", gotAgents, wantAgents)
			}

			gotJP := evalOrSelf(t, cfg.AgentsJPDir)
			wantJP := evalOrSelf(t, tt.wantJPDir(cwd))
			if gotJP != wantJP {
				t.Errorf("AgentsJPDir = %q, want %q", gotJP, wantJP)
			}

			if !slicesEqual(rest, tt.wantRemaining) {
				t.Errorf("remaining = %v, want %v", rest, tt.wantRemaining)
			}
		})
	}
}

func TestResolve_DefaultsToSkillsMode(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg, _, err := Resolve([]string{"status", "foo"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.Mode != ModeSkills {
		t.Errorf("Mode = %v, want ModeSkills", cfg.Mode)
	}
}

func TestResolveRejectsMixedModeFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "skills-dir と agents-dir の混在",
			args: []string{"--skills-dir", "/abs/skills", "--agents-dir", "/abs/agents", "status", "foo"},
		},
		{
			name: "skills-jp-dir と agents-jp-dir の混在",
			args: []string{"--skills-jp-dir", "/abs/skills-jp", "--agents-jp-dir", "/abs/agents-jp", "status", "foo"},
		},
		{
			name: "skills-dir と agents-jp-dir の混在",
			args: []string{"--skills-dir", "/abs/skills", "--agents-jp-dir", "/abs/agents-jp", "status", "foo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if _, _, err := Resolve(tt.args); err == nil {
				t.Fatal("expected error for mixed skills/agents flags")
			}
		})
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
