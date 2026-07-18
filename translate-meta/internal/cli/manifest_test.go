package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestRunManifest_ArgErrors(t *testing.T) {
	ctx := t.Context()
	cfg := Config{SkillsDir: t.TempDir(), SkillsJPDir: t.TempDir()}

	if got := RunManifest(ctx, cfg, []string{}); got != ERROR {
		t.Errorf("RunManifest(no args) = %d, want %d", got, ERROR)
	}
	if got := RunManifest(ctx, cfg, []string{"a", "b"}); got != ERROR {
		t.Errorf("RunManifest(too many) = %d, want %d", got, ERROR)
	}
}

func TestRunManifest_State(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name      string
		setup     func(t *testing.T, cfg Config, skillName, hash string)
		wantState string
	}{
		{
			name:      ".translate-meta が無い",
			setup:     func(t *testing.T, cfg Config, skillName, hash string) {},
			wantState: "untranslated",
		},
		{
			name: "ハッシュが一致する",
			setup: func(t *testing.T, cfg Config, skillName, hash string) {
				writeMeta(t, cfg, skillName, hash)
			},
			wantState: "up-to-date",
		},
		{
			name: "ハッシュが一致しない",
			setup: func(t *testing.T, cfg Config, skillName, hash string) {
				writeMeta(t, cfg, skillName, "stale-hash")
			},
			wantState: "needs-update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
			hash, err := hashDir(ctx, filepath.Join(cfg.SkillsJPDir, "foo"))
			if err != nil {
				t.Fatal(err)
			}
			tt.setup(t, cfg, "foo", hash)

			var out manifestOutput
			var code int
			stdout := captureStdout(t, func() {
				code = RunManifest(ctx, cfg, []string{"foo"})
			})
			if code != 0 {
				t.Fatalf("RunManifest = %d, want 0", code)
			}
			if err := json.Unmarshal([]byte(stdout), &out); err != nil {
				t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
			}
			if out.Skill != "foo" {
				t.Errorf("Skill = %q, want %q", out.Skill, "foo")
			}
			if out.State != tt.wantState {
				t.Errorf("State = %q, want %q", out.State, tt.wantState)
			}
		})
	}
}

func TestRunManifest_Files(t *testing.T) {
	ctx := t.Context()
	cfg := makeSkillFixture(t, "foo", map[string]string{
		"SKILL.md": "content",
		"sub/a.md": "alpha",
	})

	var out manifestOutput
	stdout := captureStdout(t, func() {
		if got := RunManifest(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunManifest = %d, want 0", got)
		}
	})
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
	}

	want := []string{"SKILL.md", "sub/a.md"}
	if !slices.Equal(out.Files, want) {
		t.Errorf("Files = %v, want %v", out.Files, want)
	}
}

func TestRunManifest_Integration(t *testing.T) {
	cfg := makeSkillFixture(t, "bar", map[string]string{"x.md": "data"})

	var out manifestOutput
	stdout := captureStdout(t, func() {
		code := Run(t.Context(), []string{
			"--skills-dir", cfg.SkillsDir,
			"--skills-jp-dir", cfg.SkillsJPDir,
			"manifest", "bar",
		})
		if code != 0 {
			t.Fatalf("Run(manifest) = %d, want 0", code)
		}
	})
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
	}
	if out.Skill != "bar" {
		t.Errorf("Skill = %q, want %q", out.Skill, "bar")
	}
	if out.State != "untranslated" {
		t.Errorf("State = %q, want %q", out.State, "untranslated")
	}
}
