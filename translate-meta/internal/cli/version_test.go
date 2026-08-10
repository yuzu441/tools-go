package cli

import (
	"testing"
)

func TestRun_VersionFlag(t *testing.T) {
	ctx := t.Context()

	for _, flag := range []string{"--version", "-version"} {
		t.Run(flag, func(t *testing.T) {
			var code int
			stdout := captureStdout(t, func() {
				code = Run(ctx, []string{flag})
			})

			if code != 0 {
				t.Errorf("Run(%q) = %d, want 0", flag, code)
			}
			if want := "translate-meta " + version + "\n"; stdout != want {
				t.Errorf("Run(%q) stdout = %q, want %q", flag, stdout, want)
			}
		})
	}
}
