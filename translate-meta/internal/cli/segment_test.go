package cli

import (
	"testing"
)

func mustSplit(t *testing.T, content string) []block {
	t.Helper()
	blocks, err := splitSegments(content)
	if err != nil {
		t.Fatalf("splitSegments: %v", err)
	}
	return blocks
}

func TestSplitSegments_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "見出しと段落が空行で区切られる",
			content: "# Heading\n\nParagraph one.\n\nParagraph two.\n",
		},
		{
			name:    "フェンスコード内の空行は境界にならない",
			content: "Intro text.\n\n```\ncode line 1\n\ncode line 2\n```\n\nOutro text.\n",
		},
		{
			name:    "フェンス文字種が異なると閉じない",
			content: "```\nsome code\n~~~\nmore code\n```\n",
		},
		{
			name:    "GFMテーブルの直後に空行なしで段落が続く",
			content: "| A | B |\n| - | - |\n| 1 | 2 |\nNot a table row, prose right after.\n",
		},
		{
			name:    "loose listは空行を挟んでも1つの塊として扱われる",
			content: "- item 1\n\n- item 2\n\n- item 3\n",
		},
		{
			name:    "setext見出し",
			content: "Heading\n=======\n\nBody text.\n",
		},
		{
			name:    "CRLF改行",
			content: "Paragraph one.\r\n\r\nParagraph two.\r\n",
		},
		{
			name:    "末尾に改行がない",
			content: "Just one line with no trailing newline",
		},
		{
			name:    "frontmatterを含む",
			content: "---\ntitle: x\n---\n\n# Heading\n\nParagraph.\n",
		},
		{
			name:    "空文字列",
			content: "",
		},
		{
			name:    "空行のみ",
			content: "\n\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, err := splitSegments(tt.content)
			if err != nil {
				t.Fatalf("splitSegments: %v", err)
			}
			got := joinSegments(blocks)
			if got != tt.content {
				t.Errorf("join(split(x)) != x\ngot:  %q\nwant: %q", got, tt.content)
			}
		})
	}
}

func TestSplitSegments_UnclosedFrontmatterFailsClosed(t *testing.T) {
	_, err := splitSegments("---\ntitle: x\nno closing fence\n")
	if err == nil {
		t.Fatal("expected error for unclosed frontmatter")
	}
}

func TestSplitSegments_BlockKinds(t *testing.T) {
	t.Run("見出しと段落が空行で区切られる", func(t *testing.T) {
		blocks := mustSplit(t, "# Heading\n\nParagraph one.\n\nParagraph two.\n")
		wantKinds := []blockKind{blockText, blockBlank, blockText, blockBlank, blockText}
		assertKinds(t, blocks, wantKinds)
	})

	t.Run("フェンスコード内の空行はcodeブロックに含まれる", func(t *testing.T) {
		blocks := mustSplit(t, "Intro.\n\n```\ncode 1\n\ncode 2\n```\n\nOutro.\n")
		wantKinds := []blockKind{blockText, blockBlank, blockCode, blockBlank, blockText}
		assertKinds(t, blocks, wantKinds)

		var code block
		for _, b := range blocks {
			if b.Kind == blockCode {
				code = b
			}
		}
		want := "```\ncode 1\n\ncode 2\n```\n"
		if code.Raw != want {
			t.Errorf("code block Raw = %q, want %q", code.Raw, want)
		}
	})

	t.Run("loose listは1つのtextブロックにまとまる", func(t *testing.T) {
		blocks := mustSplit(t, "- item 1\n\n- item 2\n\n- item 3\n")
		wantKinds := []blockKind{blockText}
		assertKinds(t, blocks, wantKinds)
	})

	t.Run("loose listでない空行区切りは境界になる", func(t *testing.T) {
		blocks := mustSplit(t, "Paragraph one.\n\nParagraph two.\n")
		wantKinds := []blockKind{blockText, blockBlank, blockText}
		assertKinds(t, blocks, wantKinds)
	})

	t.Run("frontmatterは先頭のみ検出される", func(t *testing.T) {
		blocks := mustSplit(t, "---\ntitle: x\n---\n\nBody.\n")
		if blocks[0].Kind != blockFrontmatter {
			t.Fatalf("blocks[0].Kind = %v, want blockFrontmatter", blocks[0].Kind)
		}
		if blocks[0].Raw != "---\ntitle: x\n---\n" {
			t.Errorf("frontmatter Raw = %q", blocks[0].Raw)
		}
	})

	t.Run("先頭以外の---はfrontmatterと誤認しない", func(t *testing.T) {
		blocks := mustSplit(t, "Body.\n\n---\nnot frontmatter\n")
		if blocks[0].Kind == blockFrontmatter {
			t.Errorf("blocks[0].Kind = blockFrontmatter, want not frontmatter (only valid at file start)")
		}
	})
}

func assertKinds(t *testing.T, blocks []block, want []blockKind) {
	t.Helper()
	if len(blocks) != len(want) {
		t.Fatalf("got %d blocks %v, want %d blocks %v", len(blocks), kindsOf(blocks), len(want), want)
	}
	for i, b := range blocks {
		if b.Kind != want[i] {
			t.Errorf("blocks[%d].Kind = %v, want %v (all kinds: %v)", i, b.Kind, want[i], kindsOf(blocks))
		}
	}
}

func kindsOf(blocks []block) []blockKind {
	kinds := make([]blockKind, len(blocks))
	for i, b := range blocks {
		kinds[i] = b.Kind
	}
	return kinds
}

func TestTranslatableSegments(t *testing.T) {
	blocks := mustSplit(t, "---\ntitle: x\n---\n\n# Heading\n\nParagraph.\n")
	got := translatableSegments(blocks)
	want := []string{"# Heading\n", "Paragraph.\n"}
	if len(got) != len(want) {
		t.Fatalf("translatableSegments = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("segment[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestHashSegment(t *testing.T) {
	h1 := hashSegment("hello")
	h2 := hashSegment("hello")
	h3 := hashSegment("world")

	if h1 != h2 {
		t.Errorf("hashSegment not deterministic: %q != %q", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("hashSegment collision for different input")
	}
	if h1[:7] != "sha256:" {
		t.Errorf("hashSegment = %q, want sha256: prefix", h1)
	}
}
