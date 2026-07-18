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

	want := []fileEntry{
		{Path: "SKILL.md", Policy: "copy", Reason: "english"},
		{Path: "sub/a.md", Policy: "copy", Reason: "english"},
	}
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

func TestClassifyPolicy(t *testing.T) {
	tests := []struct {
		name       string
		relPath    string
		content    string
		wantPolicy string
		wantReason string
	}{
		{
			name:       "templatesディレクトリはpreserve",
			relPath:    "templates/prompt.md",
			content:    "テンプレート本文",
			wantPolicy: "preserve",
			wantReason: "",
		},
		{
			name:       "コードファイルはcopy/code",
			relPath:    "helper.js",
			content:    "console.log('hi')",
			wantPolicy: "copy",
			wantReason: "code",
		},
		{
			name:       "日本語本文を含むmdはtranslate",
			relPath:    "SKILL.md",
			content:    "これはスキルの説明です。",
			wantPolicy: "translate",
			wantReason: "",
		},
		{
			name:       "英語のみの本文を含むmdはcopy/english",
			relPath:    "references/hono.md",
			content:    "This is an English only document.",
			wantPolicy: "copy",
			wantReason: "english",
		},
		{
			name:       "その他の拡張子はcopy/other (json)",
			relPath:    "config.json",
			content:    `{"key": "value"}`,
			wantPolicy: "copy",
			wantReason: "other",
		},
		{
			name:       "その他の拡張子はcopy/other (png)",
			relPath:    "image.png",
			content:    "binary-ish content",
			wantPolicy: "copy",
			wantReason: "other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			fullPath := filepath.Join(dir, filepath.FromSlash(tt.relPath))
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fullPath, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}

			got, err := classifyPolicy(tt.relPath, fullPath)
			if err != nil {
				t.Fatalf("classifyPolicy: %v", err)
			}
			if got.Policy != tt.wantPolicy {
				t.Errorf("Policy = %q, want %q", got.Policy, tt.wantPolicy)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}
		})
	}
}

func TestContainsJapanese(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "日本語を含む",
			text: "これはテストです。",
			want: true,
		},
		{
			name: "英語のみ",
			text: "This is a test.",
			want: false,
		},
		{
			name: "空文字",
			text: "",
			want: false,
		},
		{
			name: "カタカナのみ",
			text: "テスト",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsJapanese(tt.text); got != tt.want {
				t.Errorf("containsJapanese(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestStripNonBody(t *testing.T) {
	tests := []struct {
		name             string
		content          string
		wantJapaneseLeft bool
	}{
		{
			name: "フロントマターのみ日本語、本文は英語",
			content: `---
title: これはテストです
---
This is an English body.`,
			wantJapaneseLeft: false,
		},
		{
			name:             "フェンスコードブロックのみ日本語、本文は英語",
			content:          "This is an English body.\n```\nこれはコードです\n```\n",
			wantJapaneseLeft: false,
		},
		{
			name:             "インラインコードのみ日本語、本文は英語",
			content:          "This is `これはコードです` an English body.",
			wantJapaneseLeft: false,
		},
		{
			name:             "本文に日本語が残る",
			content:          "This body has 日本語 mixed in.",
			wantJapaneseLeft: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stripped := stripNonBody(tt.content)
			got := containsJapanese(stripped)
			if got != tt.wantJapaneseLeft {
				t.Errorf("containsJapanese(stripNonBody(%q)) = %v, want %v\nstripped: %q", tt.content, got, tt.wantJapaneseLeft, stripped)
			}
		})
	}
}

func TestListOrphans(t *testing.T) {
	t.Run("skills/にのみ存在するファイルはorphan", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		writeSkillsFile(t, cfg, "foo", "orphan.md", "orphaned")

		orphans, err := listOrphans(cfg.SkillsDir, cfg.SkillsJPDir, "foo")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(orphans, "orphan.md") {
			t.Errorf("orphans = %v, want to contain %q", orphans, "orphan.md")
		}
	})

	t.Run("DO_NOT_EDIT.txtは除外される", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		writeSkillsFile(t, cfg, "foo", "DO_NOT_EDIT.txt", "do not edit")

		orphans, err := listOrphans(cfg.SkillsDir, cfg.SkillsJPDir, "foo")
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(orphans, "DO_NOT_EDIT.txt") {
			t.Errorf("orphans = %v, want to not contain %q", orphans, "DO_NOT_EDIT.txt")
		}
	})

	t.Run(".translate-metaは除外される", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		writeMeta(t, cfg, "foo", "somehash")

		orphans, err := listOrphans(cfg.SkillsDir, cfg.SkillsJPDir, "foo")
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(orphans, ".translate-meta") {
			t.Errorf("orphans = %v, want to not contain %q", orphans, ".translate-meta")
		}
	})

	t.Run("両側に存在するファイルはorphanではない", func(t *testing.T) {
		cfg := makeSkillFixture(t, "foo", map[string]string{"a.md": "hello"})
		writeSkillsFile(t, cfg, "foo", "a.md", "hello (translated)")

		orphans, err := listOrphans(cfg.SkillsDir, cfg.SkillsJPDir, "foo")
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(orphans, "a.md") {
			t.Errorf("orphans = %v, want to not contain %q", orphans, "a.md")
		}
	})

	t.Run("skills/ディレクトリが存在しない場合は空スライス", func(t *testing.T) {
		root := t.TempDir()
		skillsDir := filepath.Join(root, "skills")
		skillsJPDir := filepath.Join(root, "skills-jp")
		if err := os.MkdirAll(filepath.Join(skillsJPDir, "foo"), 0o755); err != nil {
			t.Fatal(err)
		}

		orphans, err := listOrphans(skillsDir, skillsJPDir, "foo")
		if err != nil {
			t.Fatal(err)
		}
		if len(orphans) != 0 {
			t.Errorf("orphans = %v, want empty", orphans)
		}
	})
}

func writeSkillsFile(t *testing.T, cfg Config, skillName, name, content string) {
	t.Helper()
	fullPath := filepath.Join(cfg.SkillsDir, skillName, name)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunManifest_EndToEnd(t *testing.T) {
	cfg := makeSkillFixture(t, "mixed", map[string]string{
		"SKILL.md":          "これは日本語の説明です。",
		"references/en.md":  "English only reference.",
		"helper.js":         "console.log('hi')",
		"templates/tmpl.md": "テンプレート {{name}}",
	})
	hash, err := hashDir(t.Context(), filepath.Join(cfg.SkillsJPDir, "mixed"))
	if err != nil {
		t.Fatal(err)
	}
	writeMeta(t, cfg, "mixed", hash)
	writeSkillsFile(t, cfg, "mixed", "orphan.md", "orphaned translation")

	var out manifestOutput
	stdout := captureStdout(t, func() {
		code := Run(t.Context(), []string{
			"--skills-dir", cfg.SkillsDir,
			"--skills-jp-dir", cfg.SkillsJPDir,
			"manifest", "mixed",
		})
		if code != 0 {
			t.Fatalf("Run(manifest) = %d, want 0", code)
		}
	})
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %q", err, stdout)
	}

	if out.Skill != "mixed" {
		t.Errorf("Skill = %q, want %q", out.Skill, "mixed")
	}
	if out.State != "up-to-date" {
		t.Errorf("State = %q, want %q", out.State, "up-to-date")
	}

	wantFiles := []fileEntry{
		{Path: "SKILL.md", Policy: "translate"},
		{Path: "helper.js", Policy: "copy", Reason: "code"},
		{Path: "references/en.md", Policy: "copy", Reason: "english"},
		{Path: "templates/tmpl.md", Policy: "preserve"},
	}
	if !slices.Equal(out.Files, wantFiles) {
		t.Errorf("Files = %v, want %v", out.Files, wantFiles)
	}

	wantOrphans := []string{"orphan.md"}
	if !slices.Equal(out.Orphans, wantOrphans) {
		t.Errorf("Orphans = %v, want %v", out.Orphans, wantOrphans)
	}
}
