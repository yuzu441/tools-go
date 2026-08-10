package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// makeAgentFixture builds an agent-mode Config under a fresh temp root. The
// agents/ side is left empty unless the caller populates it separately (see
// writeAgentsFile).
func makeAgentFixture(t *testing.T, name, jpContent string) Config {
	t.Helper()
	root := t.TempDir()
	cfg := Config{
		Mode:        ModeAgents,
		AgentsDir:   filepath.Join(root, "agents"),
		AgentsJPDir: filepath.Join(root, "agents-jp"),
	}
	if err := os.MkdirAll(cfg.AgentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cfg.AgentsJPDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.AgentsJPDir, name+".md"), []byte(jpContent), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func writeAgentsFile(t *testing.T, cfg Config, relPath, content string) {
	t.Helper()
	full := filepath.Join(cfg.AgentsDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestConfig_JPRoot(t *testing.T) {
	t.Run("skillsモードはskills-jp/<name>を返す", func(t *testing.T) {
		cfg := Config{Mode: ModeSkills, SkillsJPDir: "/root/skills-jp"}
		if got, want := cfg.JPRoot("foo"), filepath.Join("/root/skills-jp", "foo"); got != want {
			t.Errorf("JPRoot = %q, want %q", got, want)
		}
	})

	t.Run("agentsモードはagents-jpそのものを返す(1階層下ではない)", func(t *testing.T) {
		cfg := Config{Mode: ModeAgents, AgentsJPDir: "/root/agents-jp"}
		if got, want := cfg.JPRoot("reviewer"), "/root/agents-jp"; got != want {
			t.Errorf("JPRoot = %q, want %q", got, want)
		}
	})
}

func TestConfig_ENRoot(t *testing.T) {
	t.Run("skillsモードはskills/<name>を返す", func(t *testing.T) {
		cfg := Config{Mode: ModeSkills, SkillsDir: "/root/skills"}
		if got, want := cfg.ENRoot("foo"), filepath.Join("/root/skills", "foo"); got != want {
			t.Errorf("ENRoot = %q, want %q", got, want)
		}
	})

	t.Run("agentsモードはagentsそのものを返す", func(t *testing.T) {
		cfg := Config{Mode: ModeAgents, AgentsDir: "/root/agents"}
		if got, want := cfg.ENRoot("reviewer"), "/root/agents"; got != want {
			t.Errorf("ENRoot = %q, want %q", got, want)
		}
	})
}

func TestConfig_MetaPath(t *testing.T) {
	t.Run("skillsモードはskills/<name>/.translate-metaを返す", func(t *testing.T) {
		cfg := Config{Mode: ModeSkills, SkillsDir: "/root/skills"}
		want := filepath.Join("/root/skills", "foo", metaFileName)
		if got := cfg.MetaPath("foo"); got != want {
			t.Errorf("MetaPath = %q, want %q", got, want)
		}
	})

	t.Run("agentsモードはagents/.translate-meta/<name>.jsonを返す", func(t *testing.T) {
		cfg := Config{Mode: ModeAgents, AgentsDir: "/root/agents"}
		want := filepath.Join("/root/agents", ".translate-meta", "reviewer.json")
		if got := cfg.MetaPath("reviewer"); got != want {
			t.Errorf("MetaPath = %q, want %q", got, want)
		}
	})
}

func TestConfig_ListFiles_AgentMode(t *testing.T) {
	t.Run("日本語本文を含むエージェントはtranslate単一エントリになる", func(t *testing.T) {
		cfg := makeAgentFixture(t, "reviewer", "あなたはレビュアーです。")

		entries, err := cfg.ListFiles("reviewer")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("ListFiles = %v, want length 1", entries)
		}
		want := fileEntry{Path: "reviewer.md", Policy: "translate"}
		if entries[0] != want {
			t.Errorf("ListFiles[0] = %+v, want %+v", entries[0], want)
		}
	})

	t.Run("英語のみのエージェントはcopy/englishになる", func(t *testing.T) {
		cfg := makeAgentFixture(t, "helper", "You are a helper agent.")

		entries, err := cfg.ListFiles("helper")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("ListFiles = %v, want length 1", entries)
		}
		want := fileEntry{Path: "helper.md", Policy: "copy", Reason: "english"}
		if entries[0] != want {
			t.Errorf("ListFiles[0] = %+v, want %+v", entries[0], want)
		}
	})

	t.Run("存在しないエージェント名はエラー", func(t *testing.T) {
		cfg := makeAgentFixture(t, "reviewer", "あなたはレビュアーです。")
		if _, err := cfg.ListFiles("missing"); err == nil {
			t.Fatal("expected error for missing agent source file")
		}
	})
}

func TestConfig_Orphans_AgentMode(t *testing.T) {
	t.Run("TM記録が無ければEN専用エージェントはorphanにならない", func(t *testing.T) {
		cfg := makeAgentFixture(t, "reviewer", "あなたはレビュアーです。")
		writeAgentsFile(t, cfg, "english-only.md", "You are an English-only agent.")

		orphans, err := cfg.Orphans("english-only")
		if err != nil {
			t.Fatal(err)
		}
		if len(orphans) != 0 {
			t.Errorf("Orphans = %v, want empty (untracked EN-only agent is not an orphan)", orphans)
		}
	})

	t.Run("JP側が両方存在すればorphanではない", func(t *testing.T) {
		cfg := makeAgentFixture(t, "reviewer", "あなたはレビュアーです。")
		writeAgentsFile(t, cfg, "reviewer.md", "You are a reviewer.")

		orphans, err := cfg.Orphans("reviewer")
		if err != nil {
			t.Fatal(err)
		}
		if len(orphans) != 0 {
			t.Errorf("Orphans = %v, want empty", orphans)
		}
	})

	t.Run("TM記録があってJPソースが消えていればorphan", func(t *testing.T) {
		cfg := makeAgentFixture(t, "reviewer", "あなたはレビュアーです。")
		writeAgentsFile(t, cfg, "reviewer.md", "You are a reviewer.")
		writeAgentMeta(t, cfg, "reviewer", metaV2{Version: 2, Files: map[string]fileMeta{
			"reviewer.md": {Policy: "translate"},
		}})

		if err := os.Remove(filepath.Join(cfg.AgentsJPDir, "reviewer.md")); err != nil {
			t.Fatal(err)
		}

		orphans, err := cfg.Orphans("reviewer")
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"reviewer.md"}
		if len(orphans) != 1 || orphans[0] != want[0] {
			t.Errorf("Orphans = %v, want %v", orphans, want)
		}
	})

	t.Run("DO_NOT_EDIT.txtと.translate-metaディレクトリ自体はそもそも対象にならない", func(t *testing.T) {
		cfg := makeAgentFixture(t, "reviewer", "あなたはレビュアーです。")
		writeAgentsFile(t, cfg, "DO_NOT_EDIT.txt", "do not edit")
		writeAgentMeta(t, cfg, "reviewer", metaV2{Version: 2, Files: map[string]fileMeta{
			"reviewer.md": {Policy: "translate"},
		}})

		orphans, err := cfg.Orphans("reviewer")
		if err != nil {
			t.Fatal(err)
		}
		if len(orphans) != 0 {
			t.Errorf("Orphans = %v, want empty (JP source still present)", orphans)
		}
	})
}

func writeAgentMeta(t *testing.T, cfg Config, name string, meta metaV2) {
	t.Helper()
	out, err := marshalMetaV2(meta)
	if err != nil {
		t.Fatal(err)
	}
	metaPath := cfg.MetaPath(name)
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, out, 0o644); err != nil {
		t.Fatal(err)
	}
}
