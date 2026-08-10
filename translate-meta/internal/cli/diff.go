package cli

// opKind classifies one element of a diffHashes result relative to the old
// and new sequences it was computed from.
type opKind int

const (
	opKept opKind = iota
	opAdded
	opDeleted
)

// diffOp is one step of a diffHashes alignment. OldIndex is meaningful for
// opKept and opDeleted; NewIndex is meaningful for opKept and opAdded.
type diffOp struct {
	Kind     opKind
	OldIndex int
	NewIndex int
}

// diffHashes computes a longest-common-subsequence diff between old and new
// (typically JP segment hashes recorded previously vs. the current file's
// segment hashes). Elements are classified as kept (unchanged, present in
// both), added (new in the current file), or deleted (removed from the
// current file). No similarity matching is attempted: an edited segment is
// represented as a deletion of the old segment plus an addition of the new
// one, never as a "modified" pairing.
//
// A positional comparison (old[i] vs new[i]) is not used deliberately: it
// would classify every element after an insertion or deletion as changed,
// which defeats the goal of only surfacing genuinely new/edited content.
func diffHashes(old, new []string) []diffOp {
	n, m := len(old), len(new)
	lcs := lcsLengths(old, new)

	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case old[i] == new[j]:
			ops = append(ops, diffOp{Kind: opKept, OldIndex: i, NewIndex: j})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, diffOp{Kind: opDeleted, OldIndex: i})
			i++
		default:
			ops = append(ops, diffOp{Kind: opAdded, NewIndex: j})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{Kind: opDeleted, OldIndex: i})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{Kind: opAdded, NewIndex: j})
	}
	return ops
}

// lcsLengths returns a (len(old)+1) x (len(new)+1) table where entry [i][j]
// is the length of the longest common subsequence of old[i:] and new[j:].
func lcsLengths(old, new []string) [][]int {
	n, m := len(old), len(new)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if old[i] == new[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	return lcs
}
