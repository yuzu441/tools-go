package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunRecord_RoundTrip(t *testing.T) {
	ctx := t.Context()
	cfg := makeSkillFixture(t, "foo", map[string]string{
		"README.md": "hello world",
		"sub/a.md":  "alpha",
	})

	if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
		t.Fatalf("RunRecord = %d, want 0", got)
	}

	metaPath := filepath.Join(cfg.SkillsDir, "foo", metaFileName)
	if _, err := os.Stat(metaPath); err != nil {
		t.Fatalf("meta file was not created: %v", err)
	}

	if got := RunStatus(ctx, cfg, []string{"foo"}); got != UP_TO_DATE {
		t.Errorf("RunStatus right after record = %d, want %d (UP_TO_DATE)", got, UP_TO_DATE)
	}
}

func TestRunRecord_DetectsDrift(t *testing.T) {
	ctx := t.Context()
	cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})

	if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
		t.Fatalf("RunRecord = %d, want 0", got)
	}

	// Mutate the skills-jp side after recording.
	if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "a.md"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := RunStatus(ctx, cfg, []string{"foo"}); got != NEEDS_UPDATE {
		t.Errorf("RunStatus after mutation = %d, want %d (NEEDS_UPDATE)", got, NEEDS_UPDATE)
	}
}

func TestRunRecord_CreatesMissingSkillsDir(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	cfg := Config{
		SkillsDir:   filepath.Join(root, "skills"),
		SkillsJPDir: filepath.Join(root, "skills-jp"),
	}
	// Create only skills-jp/<name>; invoke record while skills/<name> does not exist.
	if err := os.MkdirAll(filepath.Join(cfg.SkillsJPDir, "foo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "a.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
		t.Fatalf("RunRecord = %d, want 0", got)
	}

	metaPath := filepath.Join(cfg.SkillsDir, "foo", metaFileName)
	if _, err := os.Stat(metaPath); err != nil {
		t.Fatalf("meta file was not created: %v", err)
	}
}

func TestRunRecord_ArgErrors(t *testing.T) {
	ctx := t.Context()
	cfg := Config{SkillsDir: t.TempDir(), SkillsJPDir: t.TempDir()}

	if got := RunRecord(ctx, cfg, []string{}); got != ERROR {
		t.Errorf("RunRecord(no args) = %d, want %d", got, ERROR)
	}
	if got := RunRecord(ctx, cfg, []string{"a", "b"}); got != ERROR {
		t.Errorf("RunRecord(too many) = %d, want %d", got, ERROR)
	}
}
