package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// blockKind classifies one segment of a Markdown document produced by
// splitSegments. Only blockText is translatable; the others are carried
// through verbatim so that join(split(x)) == x always holds.
type blockKind int

const (
	blockFrontmatter blockKind = iota
	blockCode
	blockBlank
	blockText
)

// block is one contiguous run of lines with a single classification. Raw
// holds the exact original bytes (including line endings) for that run.
type block struct {
	Kind blockKind
	Raw  string
}

var listMarkerRE = regexp.MustCompile(`^ {0,3}([-*+]|\d{1,9}[.)])[ \t]`)

// splitSegments scans Markdown content into an ordered list of blocks using
// a hand-rolled line scanner (no CommonMark parser dependency, since the
// only requirement is byte-exact reconstruction via joinSegments, not
// rendering). Frontmatter, fenced code, and blank-line runs are carried
// verbatim; everything else becomes a translatable text block.
func splitSegments(content string) ([]block, error) {
	lines := splitLines(content)

	var blocks []block
	i := 0

	fmEnd, hasFrontmatter, err := scanFrontmatter(lines)
	if err != nil {
		return nil, err
	}
	if hasFrontmatter {
		blocks = append(blocks, block{blockFrontmatter, joinLines(lines[:fmEnd])})
		i = fmEnd
	}

	for i < len(lines) {
		switch {
		case isBlankLine(lines[i]):
			end := scanBlankRun(lines, i)
			blocks = append(blocks, block{blockBlank, joinLines(lines[i:end])})
			i = end
		case isFenceOpen(lines[i]):
			end := scanCodeBlock(lines, i)
			blocks = append(blocks, block{blockCode, joinLines(lines[i:end])})
			i = end
		default:
			raw, end := scanTextBlock(lines, i)
			blocks = append(blocks, block{blockText, raw})
			i = end
		}
	}

	return blocks, nil
}

func joinSegments(blocks []block) string {
	var b strings.Builder
	for _, blk := range blocks {
		b.WriteString(blk.Raw)
	}
	return b.String()
}

// translatableSegments returns the raw text of every blockText block, in
// document order. These are the only units subject to translation-memory
// tracking; frontmatter, code, and blank runs are excluded.
func translatableSegments(blocks []block) []string {
	segs := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Kind == blockText {
			segs = append(segs, b.Raw)
		}
	}
	return segs
}

// hashSegment returns a stable, prefixed content hash for a single segment
// (or whole-file content), used as jp_hash / en_hash / whole-file hash in
// the v2 meta format.
func hashSegment(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func hashSegments(segs []string) []string {
	hashes := make([]string, len(segs))
	for i, s := range segs {
		hashes[i] = hashSegment(s)
	}
	return hashes
}

// splitLines splits content into lines that each retain their own trailing
// line-ending bytes (or none, for a final line with no trailing newline),
// so that concatenating them reproduces content exactly.
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := range len(content) {
		if content[i] == '\n' {
			lines = append(lines, content[start:i+1])
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	return strings.Join(lines, "")
}

func stripLineEnding(line string) string {
	return strings.TrimRight(line, "\r\n")
}

func isBlankLine(line string) bool {
	return strings.TrimSpace(stripLineEnding(line)) == ""
}

// isListContinuationLine backs the loose-list heuristic: a blank line
// immediately followed by such a line does not end the enclosing text block.
func isListContinuationLine(line string) bool {
	return listMarkerRE.MatchString(stripLineEnding(line))
}

// scanFrontmatter detects YAML frontmatter at the very start of the
// document only. It returns ok=false (no error) if the document does not
// open with a "---" delimiter, and an error if frontmatter opens but never
// closes.
func scanFrontmatter(lines []string) (end int, ok bool, err error) {
	if len(lines) == 0 || stripLineEnding(lines[0]) != "---" {
		return 0, false, nil
	}
	for i := 1; i < len(lines); i++ {
		if stripLineEnding(lines[i]) == "---" {
			return i + 1, true, nil
		}
	}
	return 0, false, fmt.Errorf("unclosed frontmatter: no closing --- found")
}

func scanBlankRun(lines []string, start int) int {
	i := start
	for i < len(lines) && isBlankLine(lines[i]) {
		i++
	}
	return i
}

// fenceMarker reports the fence character and run length if line opens a
// fenced code block (``` or ~~~, at least 3 characters, optionally indented
// up to 3 spaces).
func fenceMarker(line string) (ch byte, length int) {
	trimmed := strings.TrimLeft(stripLineEnding(line), " ")
	if len(trimmed) < 3 {
		return 0, 0
	}
	c := trimmed[0]
	if c != '`' && c != '~' {
		return 0, 0
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == c {
		n++
	}
	if n < 3 {
		return 0, 0
	}
	return c, n
}

func isFenceOpen(line string) bool {
	ch, _ := fenceMarker(line)
	return ch != 0
}

func isClosingFence(line string, ch byte, minLen int) bool {
	trimmed := strings.TrimLeft(stripLineEnding(line), " ")
	if len(trimmed) < minLen {
		return false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == ch {
		n++
	}
	return n >= minLen && strings.TrimSpace(trimmed[n:]) == ""
}

// scanCodeBlock returns the end index (exclusive) of the fenced code block
// opening at lines[start]. An unclosed fence consumes the rest of the
// document, which still round-trips byte-exactly even though it is not
// "closed" in the CommonMark sense.
func scanCodeBlock(lines []string, start int) int {
	ch, n := fenceMarker(lines[start])
	i := start + 1
	for i < len(lines) {
		if isClosingFence(lines[i], ch, n) {
			return i + 1
		}
		i++
	}
	return i
}

// scanTextBlock consumes one translatable text block starting at start. It
// grows through blank-line runs that are immediately followed by a list
// continuation line (the loose-list heuristic), so a loose list is captured
// as a single segment instead of being split at each blank line.
func scanTextBlock(lines []string, start int) (raw string, end int) {
	var b strings.Builder
	i := scanTextRun(lines, start)
	b.WriteString(joinLines(lines[start:i]))

	for {
		blankEnd := scanBlankRun(lines, i)
		if blankEnd == i || blankEnd >= len(lines) || !isListContinuationLine(lines[blankEnd]) {
			break
		}
		b.WriteString(joinLines(lines[i:blankEnd]))
		next := scanTextRun(lines, blankEnd)
		b.WriteString(joinLines(lines[blankEnd:next]))
		i = next
	}

	return b.String(), i
}

func scanTextRun(lines []string, start int) int {
	i := start
	for i < len(lines) && !isBlankLine(lines[i]) && !isFenceOpen(lines[i]) {
		i++
	}
	return i
}
