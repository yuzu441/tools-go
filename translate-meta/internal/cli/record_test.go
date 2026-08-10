package cli

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

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

func readRecordedMeta(t *testing.T, cfg Config, skillName string) metaV2 {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(cfg.SkillsDir, skillName, metaFileName))
	if err != nil {
		t.Fatal(err)
	}
	meta, err := parseMetaV2(string(content))
	if err != nil {
		t.Fatalf("parseMetaV2: %v\ncontent:\n%s", err, content)
	}
	return meta
}

func writeSkillsFileAt(t *testing.T, cfg Config, skillName, relPath, content string) {
	t.Helper()
	full := filepath.Join(cfg.SkillsDir, skillName, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunRecord_WritesV2Meta(t *testing.T) {
	ctx := t.Context()
	cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})

	if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
		t.Fatalf("RunRecord = %d, want 0", got)
	}

	meta := readRecordedMeta(t, cfg, "foo")
	if meta.Version != 2 {
		t.Errorf("Version = %d, want 2", meta.Version)
	}
	want := fileMeta{Policy: "copy", Hash: hashSegment("hello")}
	if got := meta.Files["a.md"]; !reflect.DeepEqual(got, want) {
		t.Errorf("Files[a.md] = %+v, want %+v", got, want)
	}
}

func TestRunRecord_PreserveFileCarriesNothing(t *testing.T) {
	ctx := t.Context()
	cfg := makeSkillFixture(t, "foo", map[string]string{"templates/t.md": "テンプレート"})

	if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
		t.Fatalf("RunRecord = %d, want 0", got)
	}

	meta := readRecordedMeta(t, cfg, "foo")
	want := fileMeta{Policy: "preserve"}
	if got := meta.Files["templates/t.md"]; !reflect.DeepEqual(got, want) {
		t.Errorf("Files[templates/t.md] = %+v, want %+v", got, want)
	}
}

func TestRunRecord_TranslateFile_PositionalPairing(t *testing.T) {
	ctx := t.Context()
	jp := "見出し\n\n段落です\n"
	cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
	writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "Heading\n\nA paragraph.\n")

	if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
		t.Fatalf("RunRecord = %d, want 0", got)
	}

	meta := readRecordedMeta(t, cfg, "foo")
	fm := meta.Files["SKILL.md"]
	if fm.Policy != "translate" {
		t.Fatalf("Policy = %q, want translate", fm.Policy)
	}
	if len(fm.Segments) != 2 {
		t.Fatalf("Segments = %v, want length 2", fm.Segments)
	}

	jpBlocks, err := splitSegments(jp)
	if err != nil {
		t.Fatal(err)
	}
	jpSegs := translatableSegments(jpBlocks)
	enBlocks, err := splitSegments("Heading\n\nA paragraph.\n")
	if err != nil {
		t.Fatal(err)
	}
	enSegs := translatableSegments(enBlocks)

	for i := range jpSegs {
		want := segmentPair{JPHash: hashSegment(jpSegs[i]), ENHash: hashSegment(enSegs[i])}
		if fm.Segments[i] != want {
			t.Errorf("Segments[%d] = %+v, want %+v", i, fm.Segments[i], want)
		}
	}
}

func TestRunRecord_TranslateFile_DegradesOnSegmentCountMismatch(t *testing.T) {
	ctx := t.Context()
	jp := "見出し\n\n段落です\n"
	cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
	// English side has only one segment while Japanese has two: cannot pair.
	writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "One combined paragraph only.\n")

	stderr := captureStderr(t, func() {
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}
	})
	if stderr == "" {
		t.Error("expected a warning on stderr when segment counts mismatch")
	}

	meta := readRecordedMeta(t, cfg, "foo")
	fm := meta.Files["SKILL.md"]
	if fm.Policy != "translate" {
		t.Fatalf("Policy = %q, want translate", fm.Policy)
	}
	if len(fm.Segments) != 0 {
		t.Errorf("Segments = %v, want empty (degraded)", fm.Segments)
	}
	if fm.Hash != hashSegment(jp) {
		t.Errorf("Hash = %q, want whole-file hash of jp content", fm.Hash)
	}
}

func TestRunRecord_TranslateFile_DegradesWhenNoEnglishFileYet(t *testing.T) {
	ctx := t.Context()
	jp := "見出し\n\n段落です\n"
	cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
	// No file at all under skills/foo/SKILL.md.

	stderr := captureStderr(t, func() {
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}
	})
	if stderr == "" {
		t.Error("expected a warning on stderr when no english file exists yet")
	}

	meta := readRecordedMeta(t, cfg, "foo")
	fm := meta.Files["SKILL.md"]
	if len(fm.Segments) != 0 {
		t.Errorf("Segments = %v, want empty (degraded)", fm.Segments)
	}
	if fm.Hash != hashSegment(jp) {
		t.Errorf("Hash = %q, want whole-file hash of jp content", fm.Hash)
	}
}

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
