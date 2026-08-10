package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRunStatus_AgentMode(t *testing.T) {
	ctx := t.Context()

	t.Run("TM記録が無いとUNTRANSLATED", func(t *testing.T) {
		cfg := makeAgentFixture(t, "reviewer", "あなたはレビュアーです。")
		if got := RunStatus(ctx, cfg, []string{"reviewer"}); got != UNTRANSLATED {
			t.Errorf("RunStatus = %d, want %d (UNTRANSLATED)", got, UNTRANSLATED)
		}
	})

	t.Run("record後はUP_TO_DATE", func(t *testing.T) {
		cfg := makeAgentFixture(t, "reviewer", "あなたはレビュアーです。")
		writeAgentsFile(t, cfg, "reviewer.md", "You are a reviewer.")
		if got := RunRecord(ctx, cfg, []string{"reviewer"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}
		if got := RunStatus(ctx, cfg, []string{"reviewer"}); got != UP_TO_DATE {
			t.Errorf("RunStatus = %d, want %d (UP_TO_DATE)", got, UP_TO_DATE)
		}
	})

	t.Run("record後にJPを編集するとNEEDS_UPDATE", func(t *testing.T) {
		cfg := makeAgentFixture(t, "reviewer", "あなたはレビュアーです。")
		writeAgentsFile(t, cfg, "reviewer.md", "You are a reviewer.")
		if got := RunRecord(ctx, cfg, []string{"reviewer"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		if err := os.WriteFile(filepath.Join(cfg.AgentsJPDir, "reviewer.md"), []byte("あなたは厳格なレビュアーです。"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := RunStatus(ctx, cfg, []string{"reviewer"}); got != NEEDS_UPDATE {
			t.Errorf("RunStatus = %d, want %d (NEEDS_UPDATE)", got, NEEDS_UPDATE)
		}
	})
}

func TestRunRecord_AgentMode(t *testing.T) {
	ctx := t.Context()

	t.Run("メタはagents/.translate-meta/<name>.jsonに書かれる", func(t *testing.T) {
		cfg := makeAgentFixture(t, "reviewer", "見出し\n\n段落です\n")
		writeAgentsFile(t, cfg, "reviewer.md", "Heading\n\nA paragraph.\n")

		if got := RunRecord(ctx, cfg, []string{"reviewer"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		metaPath := filepath.Join(cfg.AgentsDir, ".translate-meta", "reviewer.json")
		content, err := os.ReadFile(metaPath)
		if err != nil {
			t.Fatalf("meta file was not created at %s: %v", metaPath, err)
		}
		meta, err := parseMetaV2(string(content))
		if err != nil {
			t.Fatalf("parseMetaV2: %v", err)
		}
		fm := meta.Files["reviewer.md"]
		if fm.Policy != "translate" {
			t.Fatalf("Policy = %q, want translate", fm.Policy)
		}
		if len(fm.Segments) != 2 {
			t.Errorf("Segments = %v, want length 2", fm.Segments)
		}

		// agents/reviewer.md itself must be untouched by record (record only
		// ever writes the meta file; assembling translated content is a
		// separate step, same as skills mode).
		enContent, err := os.ReadFile(filepath.Join(cfg.AgentsDir, "reviewer.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(enContent) != "Heading\n\nA paragraph.\n" {
			t.Errorf("agents/reviewer.md was modified by record: %q", enContent)
		}
	})

	t.Run("他のエージェントのメタファイルは書き換えない", func(t *testing.T) {
		cfg := makeAgentFixture(t, "reviewer", "見出し\n\n段落です\n")
		writeAgentsFile(t, cfg, "reviewer.md", "Heading\n\nA paragraph.\n")
		writeAgentMeta(t, cfg, "other-agent", metaV2{Version: 2, Files: map[string]fileMeta{
			"other-agent.md": {Policy: "copy", Hash: "sha256:untouched"},
		}})

		if got := RunRecord(ctx, cfg, []string{"reviewer"}); got != 0 {
			t.Fatalf("RunRecord = %d, want 0", got)
		}

		content, err := os.ReadFile(filepath.Join(cfg.AgentsDir, ".translate-meta", "other-agent.json"))
		if err != nil {
			t.Fatal(err)
		}
		meta, err := parseMetaV2(string(content))
		if err != nil {
			t.Fatal(err)
		}
		if meta.Files["other-agent.md"].Hash != "sha256:untouched" {
			t.Errorf("other-agent's meta was modified: %+v", meta.Files["other-agent.md"])
		}
	})
}

func TestRunManifest_AgentMode(t *testing.T) {
	ctx := t.Context()

	t.Run("単一ファイルのmanifestを出力する", func(t *testing.T) {
		cfg := makeAgentFixture(t, "reviewer", "あなたはレビュアーです。")

		var out manifestOutput
		stdout := captureStdout(t, func() {
			if got := RunManifest(ctx, cfg, []string{"reviewer"}); got != 0 {
				t.Fatalf("RunManifest = %d, want 0", got)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
		}

		if out.Skill != "reviewer" {
			t.Errorf("Skill = %q, want %q", out.Skill, "reviewer")
		}
		if out.State != "untranslated" {
			t.Errorf("State = %q, want %q", out.State, "untranslated")
		}
		if len(out.Files) != 1 || out.Files[0].Path != "reviewer.md" || out.Files[0].Policy != "translate" {
			t.Errorf("Files = %+v, want single translate entry for reviewer.md", out.Files)
		}
	})

	t.Run("英語専用エージェントはorphanに現れない", func(t *testing.T) {
		cfg := makeAgentFixture(t, "reviewer", "あなたはレビュアーです。")
		writeAgentsFile(t, cfg, "english-only.md", "You are an English-only agent.")

		var out manifestOutput
		stdout := captureStdout(t, func() {
			if got := RunManifest(ctx, cfg, []string{"reviewer"}); got != 0 {
				t.Fatalf("RunManifest = %d, want 0", got)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
		}
		if len(out.Orphans) != 0 {
			t.Errorf("Orphans = %v, want empty", out.Orphans)
		}
	})
}

// TestAgentSegmentTM_EndToEnd mirrors TestSegmentTM_EndToEnd (skills mode)
// for the agent single-file adapter: bootstrap via record, an isolated JP
// edit surfacing as exactly one changed segment, assembling a patch while
// preserving unchanged bytes, re-recording, and confirming status goes back
// to up-to-date. Agent mode differs only in file discovery and meta-path
// wiring, so the same lifecycle must hold with the cores unchanged.
func TestAgentSegmentTM_EndToEnd(t *testing.T) {
	ctx := t.Context()
	jp := "見出し\n\n段落です\n"
	cfg := makeAgentFixture(t, "reviewer", jp)
	writeAgentsFile(t, cfg, "reviewer.md", "Heading\n\nA paragraph.\n")

	// 1. record bootstraps the segment pairing from the existing jp/en pair.
	if got := RunRecord(ctx, cfg, []string{"reviewer"}); got != 0 {
		t.Fatalf("RunRecord(bootstrap) = %d, want 0", got)
	}
	if got := RunStatus(ctx, cfg, []string{"reviewer"}); got != UP_TO_DATE {
		t.Fatalf("RunStatus after bootstrap = %d, want %d", got, UP_TO_DATE)
	}

	// 2. Edit exactly one JP segment (insert a new paragraph in the middle).
	edited := "見出し\n\n新しい段落\n\n段落です\n"
	if err := os.WriteFile(filepath.Join(cfg.AgentsJPDir, "reviewer.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	// 3. segments status must show exactly that one changed segment.
	var statusOut segmentsStatusOutput
	stdout := captureStdout(t, func() {
		code := RunSegmentsStatus(ctx, cfg, []string{"reviewer"})
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
	patchContent := fmt.Sprintf("=== translate-meta-patch: reviewer.md #%d ===\nA new paragraph.\n", changedIndex)
	if err := os.WriteFile(patchFile, []byte(patchContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// 5. assemble must reconstruct the file, preserving unchanged bytes.
	if got := RunSegmentsAssemble(ctx, cfg, []string{"reviewer", "--patch", patchFile}); got != 0 {
		t.Fatalf("RunSegmentsAssemble = %d, want 0", got)
	}
	assembled, err := os.ReadFile(filepath.Join(cfg.AgentsDir, "reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "Heading\n\nA new paragraph.\n\nA paragraph.\n"
	if string(assembled) != want {
		t.Fatalf("assembled = %q, want %q", assembled, want)
	}

	// 6. record again, then status must report up-to-date.
	if got := RunRecord(ctx, cfg, []string{"reviewer"}); got != 0 {
		t.Fatalf("RunRecord(after assemble) = %d, want 0", got)
	}
	if got := RunStatus(ctx, cfg, []string{"reviewer"}); got != UP_TO_DATE {
		t.Fatalf("RunStatus after re-record = %d, want %d", got, UP_TO_DATE)
	}
	captureStdout(t, func() {
		if got := RunSegmentsStatus(ctx, cfg, []string{"reviewer"}); got != UP_TO_DATE {
			t.Fatalf("RunSegmentsStatus after re-record = %d, want %d", got, UP_TO_DATE)
		}
	})
}
