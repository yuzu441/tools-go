package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	defaultSkillsDirName   = "skills"
	defaultSkillsJPDirName = "skills-jp"
)

// Config holds resolved directory paths for translate-meta.
type Config struct {
	SkillsDir   string
	SkillsJPDir string
}

// Resolve parses --skills-dir / --skills-jp-dir flags from args, falling back
// to <cwd>/skills and <cwd>/skills-jp when a flag is not supplied. Both paths
// are returned as absolute paths so downstream code is unaffected by later
// changes to the working directory (notably t.Chdir in tests).
//
// Returned remaining contains the non-flag arguments (typically the
// subcommand and its operands).
func Resolve(args []string) (Config, []string, error) {
	fs := flag.NewFlagSet("translate-meta", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var skillsDir, skillsJPDir string
	fs.StringVar(&skillsDir, "skills-dir", "", "directory containing translated skills (default: <cwd>/skills)")
	fs.StringVar(&skillsJPDir, "skills-jp-dir", "", "directory containing source skills (default: <cwd>/skills-jp)")

	if err := fs.Parse(args); err != nil {
		return Config{}, nil, fmt.Errorf("parse flags: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return Config{}, nil, fmt.Errorf("get cwd: %w", err)
	}

	resolved := func(flagVal, defaultName string) (string, error) {
		v := flagVal
		if v == "" {
			v = filepath.Join(cwd, defaultName)
		}
		abs, err := filepath.Abs(v)
		if err != nil {
			return "", fmt.Errorf("resolve %q: %w", v, err)
		}
		return abs, nil
	}

	cfg := Config{}
	if cfg.SkillsDir, err = resolved(skillsDir, defaultSkillsDirName); err != nil {
		return Config{}, nil, err
	}
	if cfg.SkillsJPDir, err = resolved(skillsJPDir, defaultSkillsJPDirName); err != nil {
		return Config{}, nil, err
	}

	return cfg, fs.Args(), nil
}
