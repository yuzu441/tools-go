package cli

import (
	"reflect"
	"testing"
)

func TestDiffHashes(t *testing.T) {
	tests := []struct {
		name string
		old  []string
		new  []string
		want []diffOp
	}{
		{
			name: "変更なしは全てkept",
			old:  []string{"a", "b", "c"},
			new:  []string{"a", "b", "c"},
			want: []diffOp{
				{Kind: opKept, OldIndex: 0, NewIndex: 0},
				{Kind: opKept, OldIndex: 1, NewIndex: 1},
				{Kind: opKept, OldIndex: 2, NewIndex: 2},
			},
		},
		{
			name: "末尾への追加",
			old:  []string{"a", "b"},
			new:  []string{"a", "b", "c"},
			want: []diffOp{
				{Kind: opKept, OldIndex: 0, NewIndex: 0},
				{Kind: opKept, OldIndex: 1, NewIndex: 1},
				{Kind: opAdded, NewIndex: 2},
			},
		},
		{
			name: "末尾の削除",
			old:  []string{"a", "b", "c"},
			new:  []string{"a", "b"},
			want: []diffOp{
				{Kind: opKept, OldIndex: 0, NewIndex: 0},
				{Kind: opKept, OldIndex: 1, NewIndex: 1},
				{Kind: opDeleted, OldIndex: 2},
			},
		},
		{
			name: "編集はdelete+addとして表現される",
			old:  []string{"a", "b", "c"},
			new:  []string{"a", "x", "c"},
			want: []diffOp{
				{Kind: opKept, OldIndex: 0, NewIndex: 0},
				{Kind: opDeleted, OldIndex: 1},
				{Kind: opAdded, NewIndex: 1},
				{Kind: opKept, OldIndex: 2, NewIndex: 2},
			},
		},
		{
			name: "中間挿入は後続セグメントをchangedにしない",
			old:  []string{"h1", "h2", "h3", "h4"},
			new:  []string{"h1", "hNEW", "h2", "h3", "h4"},
			want: []diffOp{
				{Kind: opKept, OldIndex: 0, NewIndex: 0},
				{Kind: opAdded, NewIndex: 1},
				{Kind: opKept, OldIndex: 1, NewIndex: 2},
				{Kind: opKept, OldIndex: 2, NewIndex: 3},
				{Kind: opKept, OldIndex: 3, NewIndex: 4},
			},
		},
		{
			name: "old側が空なら全てadded(新規ブートストラップ相当)",
			old:  []string{},
			new:  []string{"a", "b"},
			want: []diffOp{
				{Kind: opAdded, NewIndex: 0},
				{Kind: opAdded, NewIndex: 1},
			},
		},
		{
			name: "new側が空なら全てdeleted",
			old:  []string{"a", "b"},
			new:  []string{},
			want: []diffOp{
				{Kind: opDeleted, OldIndex: 0},
				{Kind: opDeleted, OldIndex: 1},
			},
		},
		{
			name: "両方空なら差分なし",
			old:  []string{},
			new:  []string{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffHashes(tt.old, tt.new)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("diffHashes(%v, %v) =\n%v\nwant\n%v", tt.old, tt.new, got, tt.want)
			}
		})
	}
}

// TestDiffHashes_MidFileInsertionDoesNotCascade guards the property that
// segment-level translation memory exists for: inserting a paragraph in the
// middle of a file must not mark every later segment as changed.
func TestDiffHashes_MidFileInsertionDoesNotCascade(t *testing.T) {
	old := []string{"h1", "h2", "h3", "h4", "h5"}
	new := []string{"h1", "h2", "hNEW", "h3", "h4", "h5"}

	ops := diffHashes(old, new)

	keptCount := 0
	addedCount := 0
	for _, op := range ops {
		switch op.Kind {
		case opKept:
			keptCount++
		case opAdded:
			addedCount++
		case opDeleted:
			t.Errorf("unexpected deletion for pure insertion: %+v", op)
		}
	}
	if keptCount != len(old) {
		t.Errorf("kept count = %d, want %d (all original segments should remain kept)", keptCount, len(old))
	}
	if addedCount != 1 {
		t.Errorf("added count = %d, want 1 (only the inserted segment)", addedCount)
	}
}
