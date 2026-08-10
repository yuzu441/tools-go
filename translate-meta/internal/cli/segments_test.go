package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePatch(t *testing.T) {
	t.Run("複数セグメント・複数ファイルを解析できる", func(t *testing.T) {
		content := "=== translate-meta-patch: SKILL.md #3 ===\n" +
			"New English paragraph.\n" +
			"- item 1\n" +
			"- item 2\n" +
			"=== translate-meta-patch: SKILL.md #8 ===\n" +
			"Updated paragraph with `code`.\n" +
			"=== translate-meta-patch: references/react.md #1 ===\n" +
			"Reference text.\n"

		got, err := parsePatch(content)
		if err != nil {
			t.Fatalf("parsePatch: %v", err)
		}

		want := map[patchKey]string{
			{Path: "SKILL.md", Index: 3}:            "New English paragraph.\n- item 1\n- item 2\n",
			{Path: "SKILL.md", Index: 8}:            "Updated paragraph with `code`.\n",
			{Path: "references/react.md", Index: 1}: "Reference text.\n",
		}
		if len(got) != len(want) {
			t.Fatalf("parsePatch = %v, want %v", got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("parsePatch[%v] = %q, want %q", k, got[k], v)
			}
		}
	})

	t.Run("マーカーの無い先頭コンテンツはエラー", func(t *testing.T) {
		if _, err := parsePatch("no marker here\n"); err == nil {
			t.Error("expected error for content with no markers")
		}
	})

	t.Run("同じキーの重複マーカーはエラー", func(t *testing.T) {
		content := "=== translate-meta-patch: SKILL.md #1 ===\n" +
			"first\n" +
			"=== translate-meta-patch: SKILL.md #1 ===\n" +
			"second\n"
		if _, err := parsePatch(content); err == nil {
			t.Error("expected error for duplicate patch key")
		}
	})

	t.Run("空文字列はエラー", func(t *testing.T) {
		if _, err := parsePatch(""); err == nil {
			t.Error("expected error for empty patch")
		}
	})
}

func TestRunSegmentsStatus(t *testing.T) {
	ctx := t.Context()

	t.Run("記録が無いとneeds-bootstrap", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": "見出し\n\n段落です\n"})

		var out segmentsStatusOutput
		stdout := captureStdout(t, func() {
			code := RunSegmentsStatus(ctx, cfg, []string{"foo"})
			if code == UP_TO_DATE {
				t.Fatalf("RunSegmentsStatus = %d, want non-zero for needs-bootstrap", code)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
		}
		if out.State != "needs-bootstrap" {
			t.Errorf("State = %q, want needs-bootstrap", out.State)
		}
	})

	t.Run("legacyメタもneeds-bootstrap", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": "見出し\n\n段落です\n"})
		hash, err := hashDir(ctx, filepath.Join(cfg.SkillsJPDir, "foo"))
		if err != nil {
			t.Fatal(err)
		}
		writeMeta(t, cfg, "foo", hash)

		var out segmentsStatusOutput
		stdout := captureStdout(t, func() {
			RunSegmentsStatus(ctx, cfg, []string{"foo"})
		})
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
		}
		if out.State != "needs-bootstrap" {
			t.Errorf("State = %q, want needs-bootstrap", out.State)
		}
	})

	t.Run("変更が無ければup-to-date", func(t *testing.T) {
		jp := "見出し\n\n段落です\n"
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
		writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "Heading\n\nA paragraph.\n")
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		var out segmentsStatusOutput
		stdout := captureStdout(t, func() {
			code := RunSegmentsStatus(ctx, cfg, []string{"foo"})
			if code != UP_TO_DATE {
				t.Fatalf("RunSegmentsStatus = %d, want %d", code, UP_TO_DATE)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
		}
		if out.State != "up-to-date" {
			t.Errorf("State = %q, want up-to-date", out.State)
		}
		if len(out.Files) != 1 {
			t.Fatalf("Files = %v, want length 1", out.Files)
		}
		if len(out.Files[0].Changed) != 0 {
			t.Errorf("Changed = %v, want empty", out.Files[0].Changed)
		}
		if out.Files[0].UnchangedCount != 2 {
			t.Errorf("UnchangedCount = %d, want 2", out.Files[0].UnchangedCount)
		}
	})

	t.Run("JPセグメント追加はchangedに現れる(1件のみ)", func(t *testing.T) {
		jp := "見出し\n\n段落です\n"
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
		writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "Heading\n\nA paragraph.\n")
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		// Edit the Japanese source: insert a new paragraph in the middle.
		edited := "見出し\n\n新しい段落\n\n段落です\n"
		if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "SKILL.md"), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}

		var out segmentsStatusOutput
		stdout := captureStdout(t, func() {
			code := RunSegmentsStatus(ctx, cfg, []string{"foo"})
			if code != NEEDS_UPDATE {
				t.Fatalf("RunSegmentsStatus = %d, want %d", code, NEEDS_UPDATE)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
		}
		if len(out.Files) != 1 {
			t.Fatalf("Files = %v, want length 1", out.Files)
		}
		f := out.Files[0]
		if len(f.Changed) != 1 {
			t.Fatalf("Changed = %v, want exactly 1 entry", f.Changed)
		}
		if f.Changed[0].Index != 1 {
			t.Errorf("Changed[0].Index = %d, want 1", f.Changed[0].Index)
		}
		if f.Changed[0].JPText != "新しい段落\n" {
			t.Errorf("Changed[0].JPText = %q, want %q", f.Changed[0].JPText, "新しい段落\n")
		}
		// The two original segments remain kept, not flagged as changed.
		if f.UnchangedCount != 2 {
			t.Errorf("UnchangedCount = %d, want 2 (unrelated segments must not cascade)", f.UnchangedCount)
		}
	})

	t.Run("EN直接編集はdirect_editsに現れる", func(t *testing.T) {
		jp := "見出し\n\n段落です\n"
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
		writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "Heading\n\nA paragraph.\n")
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		// Someone hand-edits the English file directly, without touching JP.
		if err := os.WriteFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"), []byte("Heading\n\nA directly edited paragraph.\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		var out segmentsStatusOutput
		stdout := captureStdout(t, func() {
			RunSegmentsStatus(ctx, cfg, []string{"foo"})
		})
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
		}
		f := out.Files[0]
		if len(f.DirectEdits) != 1 {
			t.Fatalf("DirectEdits = %v, want exactly 1 entry", f.DirectEdits)
		}
		if f.DirectEdits[0].Index != 1 {
			t.Errorf("DirectEdits[0].Index = %d, want 1", f.DirectEdits[0].Index)
		}
		if f.DirectEdits[0].ENText != "A directly edited paragraph.\n" {
			t.Errorf("DirectEdits[0].ENText = %q", f.DirectEdits[0].ENText)
		}
	})

	t.Run("degrade(whole-fileハッシュ)状態のファイルは全セグメントがaddedとして報告される", func(t *testing.T) {
		jp := "見出し\n\n段落です\n"
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
		// English side has only one segment while Japanese has two, so
		// record cannot pair them and degrades this entry to a whole-file
		// hash (see TestRunRecord_TranslateFile_DegradesOnSegmentCountMismatch).
		writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "One combined paragraph only.\n")
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		var out segmentsStatusOutput
		stdout := captureStdout(t, func() {
			code := RunSegmentsStatus(ctx, cfg, []string{"foo"})
			if code != NEEDS_UPDATE {
				t.Fatalf("RunSegmentsStatus = %d, want %d", code, NEEDS_UPDATE)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
		}
		f := out.Files[0]
		if len(f.Changed) != 2 {
			t.Fatalf("Changed = %v, want 2 entries (no recorded segments to diff against)", f.Changed)
		}
		if len(f.DirectEdits) != 0 {
			t.Errorf("DirectEdits = %v, want empty", f.DirectEdits)
		}
		if f.UnchangedCount != 0 {
			t.Errorf("UnchangedCount = %d, want 0", f.UnchangedCount)
		}
	})
}

// TestSegmentTM_CodeBlocks covers the three regressions that follow from
// treating fenced code blocks as untranslated, verbatim-copied blocks: an
// assembled code block reverting to the Japanese master's bytes, a hand
// edit inside an English code block going unreported, and an edit to a
// Japanese code block being silently treated as up-to-date.
func TestSegmentTM_CodeBlocks(t *testing.T) {
	ctx := t.Context()

	t.Run("未変更コードブロックはassembleでEN側のバイト列が保持される(JPコメントに戻らない)", func(t *testing.T) {
		jp := "見出し\n\n```python\n# コメント\nprint(1)\n```\n\n段落です\n"
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
		enCode := "```python\n# comment\nprint(1)\n```\n"
		writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "Heading\n\n"+enCode+"\nA paragraph.\n")
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		// Insert a new JP paragraph; the code block itself is untouched.
		edited := "見出し\n\n```python\n# コメント\nprint(1)\n```\n\n新しい段落\n\n段落です\n"
		if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "SKILL.md"), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}

		var statusOut segmentsStatusOutput
		stdout := captureStdout(t, func() {
			RunSegmentsStatus(ctx, cfg, []string{"foo"})
		})
		if err := json.Unmarshal([]byte(stdout), &statusOut); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
		}
		if len(statusOut.Files) != 1 || len(statusOut.Files[0].Changed) != 1 {
			t.Fatalf("expected exactly one changed segment, got %+v", statusOut.Files)
		}
		changedIndex := statusOut.Files[0].Changed[0].Index

		patchFile := filepath.Join(t.TempDir(), "patch.txt")
		patchContent := fmt.Sprintf("=== translate-meta-patch: SKILL.md #%d ===\nA new paragraph.\n", changedIndex)
		if err := os.WriteFile(patchFile, []byte(patchContent), 0o644); err != nil {
			t.Fatal(err)
		}

		if got := RunSegmentsAssemble(ctx, cfg, []string{"foo", "--patch", patchFile}); got != 0 {
			t.Fatalf("RunSegmentsAssemble = %d, want 0", got)
		}

		assembled, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		want := "Heading\n\n```python\n# comment\nprint(1)\n```\n\nA new paragraph.\n\nA paragraph.\n"
		if string(assembled) != want {
			t.Errorf("assembled = %q, want %q (the code block must keep the English file's bytes, not the Japanese source's)", assembled, want)
		}
	})

	t.Run("EN側コードブロックの直接編集はdirect_editsに現れる", func(t *testing.T) {
		jp := "見出し\n\n```python\nprint(1)\n```\n\n段落です\n"
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
		writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "Heading\n\n```python\nprint(1)\n```\n\nA paragraph.\n")
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		// Hand-edit the English code block directly, without touching JP.
		handEdited := "Heading\n\n```python\nprint(2)  # hand edited\n```\n\nA paragraph.\n"
		if err := os.WriteFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"), []byte(handEdited), 0o644); err != nil {
			t.Fatal(err)
		}

		var out segmentsStatusOutput
		stdout := captureStdout(t, func() {
			RunSegmentsStatus(ctx, cfg, []string{"foo"})
		})
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
		}
		f := out.Files[0]
		if len(f.DirectEdits) != 1 {
			t.Fatalf("DirectEdits = %v, want exactly 1 entry", f.DirectEdits)
		}
		if f.DirectEdits[0].Index != 1 {
			t.Errorf("DirectEdits[0].Index = %d, want 1", f.DirectEdits[0].Index)
		}
		wantENText := "```python\nprint(2)  # hand edited\n```\n"
		if f.DirectEdits[0].ENText != wantENText {
			t.Errorf("DirectEdits[0].ENText = %q, want %q", f.DirectEdits[0].ENText, wantENText)
		}
	})

	t.Run("JP側コードブロックの編集はchangedとして検出される(サイレントにup-to-dateにならない)", func(t *testing.T) {
		jp := "見出し\n\n```python\nprint(1)\n```\n\n段落です\n"
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
		writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "Heading\n\n```python\nprint(1)\n```\n\nA paragraph.\n")
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		// Edit only the JP code block's content; the surrounding text is untouched.
		edited := "見出し\n\n```python\nprint(999)\n```\n\n段落です\n"
		if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "SKILL.md"), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}

		var out segmentsStatusOutput
		stdout := captureStdout(t, func() {
			code := RunSegmentsStatus(ctx, cfg, []string{"foo"})
			if code != NEEDS_UPDATE {
				t.Fatalf("RunSegmentsStatus = %d, want %d (NEEDS_UPDATE); a JP code block edit must not be silently up-to-date", code, NEEDS_UPDATE)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
		}
		f := out.Files[0]
		if len(f.Changed) != 1 {
			t.Fatalf("Changed = %v, want exactly 1 entry", f.Changed)
		}
		if f.Changed[0].Index != 1 {
			t.Errorf("Changed[0].Index = %d, want 1", f.Changed[0].Index)
		}
		wantJPText := "```python\nprint(999)\n```\n"
		if f.Changed[0].JPText != wantJPText {
			t.Errorf("Changed[0].JPText = %q, want %q", f.Changed[0].JPText, wantJPText)
		}
		if f.UnchangedCount != 2 {
			t.Errorf("UnchangedCount = %d, want 2 (heading and paragraph unaffected)", f.UnchangedCount)
		}
	})
}

func TestRunSegmentsAssemble(t *testing.T) {
	ctx := t.Context()

	t.Run("パッチが不完全なら何も書かずエラー", func(t *testing.T) {
		jp := "見出し\n\n段落です\n"
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
		writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "Heading\n\nA paragraph.\n")
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		edited := "見出し\n\n新しい段落\n\n段落です\n"
		if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "SKILL.md"), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}

		originalEN, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}

		patchFile := filepath.Join(t.TempDir(), "patch.txt")
		// Patch omits the required #1 entry.
		if err := os.WriteFile(patchFile, []byte("=== translate-meta-patch: OTHER.md #0 ===\nirrelevant\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		if got := RunSegmentsAssemble(ctx, cfg, []string{"foo", "--patch", patchFile}); got == 0 {
			t.Fatalf("RunSegmentsAssemble = %d, want non-zero for incomplete patch", got)
		}

		afterEN, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(afterEN) != string(originalEN) {
			t.Errorf("english file was modified despite incomplete patch:\nbefore: %q\nafter:  %q", originalEN, afterEN)
		}
	})

	t.Run("EN直接編集はkeptセグメントとしてそのまま保持される(ブロックしない)", func(t *testing.T) {
		jp := "見出し\n\n段落です\n"
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
		writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "Heading\n\nA paragraph.\n")
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		// Hand-edit EN's first segment directly, without touching the
		// corresponding JP segment. This is a direct edit that segments
		// status surfaces for human judgment; assemble itself must not
		// block on it or overwrite it, since there is nothing to reconcile
		// against (no EN text is ever stored in the meta).
		if err := os.WriteFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"), []byte("Directly Edited Heading\n\nA paragraph.\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		// Separately (and unrelated to the direct edit above), insert a new
		// JP paragraph in the middle: this is the only segment that
		// genuinely needs a patch entry.
		if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "SKILL.md"), []byte("見出し\n\n新しい段落\n\n段落です\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		patchFile := filepath.Join(t.TempDir(), "patch.txt")
		if err := os.WriteFile(patchFile, []byte("=== translate-meta-patch: SKILL.md #1 ===\nA new paragraph.\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		if got := RunSegmentsAssemble(ctx, cfg, []string{"foo", "--patch", patchFile}); got != 0 {
			t.Fatalf("RunSegmentsAssemble = %d, want 0", got)
		}

		got, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		want := "Directly Edited Heading\n\nA new paragraph.\n\nA paragraph.\n"
		if string(got) != want {
			t.Errorf("assembled = %q, want the direct edit preserved as-is alongside the patched segment: %q", got, want)
		}
	})

	t.Run("セグメント本文中のマーカー類似行(範囲外index)はcollisionとしてエラーになり何も書かれない", func(t *testing.T) {
		jp := "見出し\n\n段落です\n"
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
		writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "Heading\n\nA paragraph.\n")
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		edited := "見出し\n\n新しい段落\n\n段落です\n"
		if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "SKILL.md"), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}

		originalEN, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}

		// The intended body for the new segment (#1) accidentally contains a
		// line matching the marker syntax with an out-of-range index (#99),
		// simulating a body/marker collision that would otherwise silently
		// truncate the real segment and go undetected.
		patchFile := filepath.Join(t.TempDir(), "patch.txt")
		patchContent := "=== translate-meta-patch: SKILL.md #1 ===\n" +
			"A new paragraph that happens to include:\n" +
			"=== translate-meta-patch: SKILL.md #99 ===\n" +
			"some trailing text\n"
		if err := os.WriteFile(patchFile, []byte(patchContent), 0o644); err != nil {
			t.Fatal(err)
		}

		if got := RunSegmentsAssemble(ctx, cfg, []string{"foo", "--patch", patchFile}); got == 0 {
			t.Fatalf("RunSegmentsAssemble = %d, want non-zero for marker collision", got)
		}

		afterEN, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(afterEN) != string(originalEN) {
			t.Errorf("english file was modified despite marker collision:\nbefore: %q\nafter:  %q", originalEN, afterEN)
		}
	})

	t.Run("セグメント本文中のマーカー類似行(範囲内だが不要なindex)もcollisionとしてエラーになり何も書かれない", func(t *testing.T) {
		// This is the more dangerous sub-case: the collision's embedded
		// index (#0) is a real, currently-kept segment that needs no
		// patching at all. A validator that only checks "is this index
		// in range" would miss it, silently truncating the real segment
		// (#1) at the collision line while writing an unused #0 entry that
		// looks harmless. The guard must reject any patch entry that
		// assemble does not actually need, not just out-of-range ones.
		jp := "見出し\n\n段落です\n"
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
		writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "Heading\n\nA paragraph.\n")
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		edited := "見出し\n\n新しい段落\n\n段落です\n"
		if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "SKILL.md"), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}

		originalEN, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}

		patchFile := filepath.Join(t.TempDir(), "patch.txt")
		patchContent := "=== translate-meta-patch: SKILL.md #1 ===\n" +
			"A new paragraph that happens to include:\n" +
			"=== translate-meta-patch: SKILL.md #0 ===\n" +
			"some trailing text\n"
		if err := os.WriteFile(patchFile, []byte(patchContent), 0o644); err != nil {
			t.Fatal(err)
		}

		if got := RunSegmentsAssemble(ctx, cfg, []string{"foo", "--patch", patchFile}); got == 0 {
			t.Fatalf("RunSegmentsAssemble = %d, want non-zero for marker collision on an in-range but unrequested index", got)
		}

		afterEN, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(afterEN) != string(originalEN) {
			t.Errorf("english file was modified despite marker collision:\nbefore: %q\nafter:  %q", originalEN, afterEN)
		}
	})

	t.Run("複数ファイルに欠損があれば全ファイル分が報告され何も書かれない", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{
			"SKILL.md": "見出し\n\n段落です\n",
			"other.md": "サブ見出し\n\nサブ段落\n",
			"third.md": "サード見出し\n\nサード段落\n",
		})
		writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "Heading\n\nA paragraph.\n")
		writeSkillsFileAt(t, cfg, "foo", "other.md", "Sub heading\n\nSub paragraph.\n")
		writeSkillsFileAt(t, cfg, "foo", "third.md", "Third heading\n\nThird paragraph.\n")
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "SKILL.md"), []byte("見出し\n\n新しい段落\n\n段落です\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "other.md"), []byte("サブ見出し\n\n新しいサブ段落\n\nサブ段落\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		// third.md also gains a new segment, but its patch entry below is
		// supplied in full: this is the file that satisfies parsePatch's
		// "at least one marker" rule with a genuinely required entry,
		// without accidentally satisfying (or colliding with) either of
		// the other two files' gaps.
		if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "third.md"), []byte("サード見出し\n\n新しいサード段落\n\nサード段落\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		originalSkillEN, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		originalOtherEN, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "other.md"))
		if err != nil {
			t.Fatal(err)
		}

		// The patch fully satisfies third.md but omits both SKILL.md's and
		// other.md's actually-added segment (#1 in each). RunSegmentsAssemble
		// must collect and report the gap in *both* files, not just the
		// first one it inspects, and must not write third.md either despite
		// it being individually complete.
		patchFile := filepath.Join(t.TempDir(), "patch.txt")
		if err := os.WriteFile(patchFile, []byte("=== translate-meta-patch: third.md #1 ===\nA new third paragraph.\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		stderr := captureStderr(t, func() {
			if got := RunSegmentsAssemble(ctx, cfg, []string{"foo", "--patch", patchFile}); got == 0 {
				t.Fatalf("RunSegmentsAssemble = %d, want non-zero for incomplete patch", got)
			}
		})
		if !strings.Contains(stderr, "SKILL.md #1") {
			t.Errorf("stderr missing report for SKILL.md #1:\n%s", stderr)
		}
		if !strings.Contains(stderr, "other.md #1") {
			t.Errorf("stderr missing report for other.md #1:\n%s", stderr)
		}

		afterSkillEN, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		afterOtherEN, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "other.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(afterSkillEN) != string(originalSkillEN) {
			t.Errorf("SKILL.md was modified despite incomplete patch")
		}
		if string(afterOtherEN) != string(originalOtherEN) {
			t.Errorf("other.md was modified despite incomplete patch")
		}
		if _, err := os.Stat(filepath.Join(cfg.SkillsDir, "foo", "third.md")); err != nil {
			t.Fatal(err)
		}
		afterThirdEN, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "third.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(afterThirdEN) != "Third heading\n\nThird paragraph.\n" {
			t.Errorf("third.md was modified despite the overall patch being incomplete")
		}
	})

	t.Run("完全なパッチで未変更セグメントはバイト保存され新規セグメントは差し込まれる", func(t *testing.T) {
		jp := "見出し\n\n段落です\n"
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
		writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "Heading\n\nA paragraph.\n")
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		edited := "見出し\n\n新しい段落\n\n段落です\n"
		if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "SKILL.md"), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}

		patchFile := filepath.Join(t.TempDir(), "patch.txt")
		if err := os.WriteFile(patchFile, []byte("=== translate-meta-patch: SKILL.md #1 ===\nA new paragraph.\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		if got := RunSegmentsAssemble(ctx, cfg, []string{"foo", "--patch", patchFile}); got != 0 {
			t.Fatalf("RunSegmentsAssemble = %d, want 0", got)
		}

		got, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		want := "Heading\n\nA new paragraph.\n\nA paragraph.\n"
		if string(got) != want {
			t.Errorf("assembled = %q, want %q", got, want)
		}
	})

	t.Run("セグメント数の増減が相殺されるdelete+insert編集でもassembleは冪等", func(t *testing.T) {
		// Deleting the first segment and appending a new one leaves the
		// total segment count unchanged (3 -> 3). A shape heuristic based
		// purely on comparing aggregate segment counts cannot tell "still
		// shaped like what was recorded" apart from "already reshaped by a
		// prior un-recorded assemble run" in this case, since both
		// candidate counts collide. Re-running assemble against its own
		// prior output (no record in between) must still be byte-identical.
		cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": "あ\n\nい\n\nう\n"})
		writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "a\n\nb\n\nc\n")
		if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		edited := "い\n\nう\n\nえ\n"
		if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "SKILL.md"), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}

		patchFile := filepath.Join(t.TempDir(), "patch.txt")
		if err := os.WriteFile(patchFile, []byte("=== translate-meta-patch: SKILL.md #2 ===\nd\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		if got := RunSegmentsAssemble(ctx, cfg, []string{"foo", "--patch", patchFile}); got != 0 {
			t.Fatalf("RunSegmentsAssemble (first run) = %d, want 0", got)
		}
		first, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		want := "b\n\nc\n\nd\n"
		if string(first) != want {
			t.Fatalf("first assemble = %q, want %q", first, want)
		}

		if got := RunSegmentsAssemble(ctx, cfg, []string{"foo", "--patch", patchFile}); got != 0 {
			t.Fatalf("RunSegmentsAssemble (second run) = %d, want 0", got)
		}
		second, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(second) != string(first) {
			t.Errorf("assemble is not idempotent for a net-zero segment-count edit: first = %q, second = %q", first, second)
		}
	})
}

// TestSegmentTM_EndToEnd covers the full lifecycle: bootstrap via record, an
// isolated JP edit surfacing as exactly one changed segment, assembling a
// patch while preserving unchanged bytes, re-recording, and confirming
// status goes back to up-to-date.
func TestSegmentTM_EndToEnd(t *testing.T) {
	ctx := t.Context()
	jp := "見出し\n\n段落です\n"
	cfg := makeSkillFixture(t, "foo", map[string]string{"SKILL.md": jp})
	writeSkillsFileAt(t, cfg, "foo", "SKILL.md", "Heading\n\nA paragraph.\n")

	// 1. record bootstraps the segment pairing from the existing jp/en pair.
	if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
		t.Fatalf("RunRecord(bootstrap) = %d, want 0", got)
	}
	if got := RunStatus(ctx, cfg, []string{"foo"}); got != UP_TO_DATE {
		t.Fatalf("RunStatus after bootstrap = %d, want %d", got, UP_TO_DATE)
	}

	// 2. Edit exactly one JP segment (insert a new paragraph in the middle).
	edited := "見出し\n\n新しい段落\n\n段落です\n"
	if err := os.WriteFile(filepath.Join(cfg.SkillsJPDir, "foo", "SKILL.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	// 3. segments status must show exactly that one changed segment.
	var statusOut segmentsStatusOutput
	stdout := captureStdout(t, func() {
		code := RunSegmentsStatus(ctx, cfg, []string{"foo"})
		if code != NEEDS_UPDATE {
			t.Fatalf("RunSegmentsStatus = %d, want %d", code, NEEDS_UPDATE)
		}
	})
	if err := json.Unmarshal([]byte(stdout), &statusOut); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
	}
	if len(statusOut.Files) != 1 || len(statusOut.Files[0].Changed) != 1 {
		t.Fatalf("expected exactly one changed segment, got %+v", statusOut.Files)
	}
	changedIndex := statusOut.Files[0].Changed[0].Index

	// 4. Write a patch translating only that segment.
	patchFile := filepath.Join(t.TempDir(), "patch.txt")
	patchContent := fmt.Sprintf("=== translate-meta-patch: SKILL.md #%d ===\nA new paragraph.\n", changedIndex)
	if err := os.WriteFile(patchFile, []byte(patchContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// 5. assemble must reconstruct the file, preserving unchanged bytes.
	if got := RunSegmentsAssemble(ctx, cfg, []string{"foo", "--patch", patchFile}); got != 0 {
		t.Fatalf("RunSegmentsAssemble = %d, want 0", got)
	}
	assembled, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "Heading\n\nA new paragraph.\n\nA paragraph.\n"
	if string(assembled) != want {
		t.Fatalf("assembled = %q, want %q", assembled, want)
	}

	// 5b. assemble is idempotent: re-running against the same patch (before
	// re-recording) must produce byte-identical output.
	if got := RunSegmentsAssemble(ctx, cfg, []string{"foo", "--patch", patchFile}); got != 0 {
		t.Fatalf("RunSegmentsAssemble (second run) = %d, want 0", got)
	}
	reassembled, err := os.ReadFile(filepath.Join(cfg.SkillsDir, "foo", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(reassembled) != string(assembled) {
		t.Fatalf("assemble is not idempotent: first = %q, second = %q", assembled, reassembled)
	}

	// 6. record again, then status must report up-to-date.
	if got := RunRecord(ctx, cfg, []string{"foo"}); got != 0 {
		t.Fatalf("RunRecord(after assemble) = %d, want 0", got)
	}
	if got := RunStatus(ctx, cfg, []string{"foo"}); got != UP_TO_DATE {
		t.Fatalf("RunStatus after re-record = %d, want %d", got, UP_TO_DATE)
	}
	captureStdout(t, func() {
		if got := RunSegmentsStatus(ctx, cfg, []string{"foo"}); got != UP_TO_DATE {
			t.Fatalf("RunSegmentsStatus after re-record = %d, want %d", got, UP_TO_DATE)
		}
	})
}
