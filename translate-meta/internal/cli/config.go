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
	defaultAgentsDirName   = "agents"
	defaultAgentsJPDirName = "agents-jp"
)

// Mode selects which kind of target translate-meta operates on: a skill
// (a directory of many files under skills-jp/) or an agent (a single file
// under agents-jp/). ModeSkills is deliberately the zero value, so a Config
// literal that never mentions Mode behaves as a skills-mode config.
type Mode int

const (
	ModeSkills Mode = iota
	ModeAgents
)

// Config holds resolved directory paths for translate-meta. Exactly one of
// the skills-* or agents-* path pairs is meaningful, per Mode.
type Config struct {
	SkillsDir   string
	SkillsJPDir string
	AgentsDir   string
	AgentsJPDir string
	Mode        Mode
}

// Resolve parses --skills-dir/--skills-jp-dir and --agents-dir/--agents-jp-dir
// flags from args. The two flag pairs are mutually exclusive: supplying any
// skills-* flag alongside any agents-* flag is an error. Mode is ModeAgents
// only when at least one agents-* flag is present; otherwise it is
// ModeSkills, with skills-dir/skills-jp-dir falling back to <cwd>/skills and
// <cwd>/skills-jp. All resolved paths are absolute so downstream code is
// unaffected by later changes to the working directory (notably t.Chdir in
// tests).
//
// Returned remaining contains the non-flag arguments (typically the
// subcommand and its operands).
func Resolve(args []string) (Config, []string, error) {
	fs := flag.NewFlagSet("translate-meta", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var skillsDir, skillsJPDir, agentsDir, agentsJPDir string
	fs.StringVar(&skillsDir, "skills-dir", "", "directory containing translated skills (default: <cwd>/skills)")
	fs.StringVar(&skillsJPDir, "skills-jp-dir", "", "directory containing source skills (default: <cwd>/skills-jp)")
	fs.StringVar(&agentsDir, "agents-dir", "", "directory containing translated agents (default: <cwd>/agents); mutually exclusive with --skills-dir/--skills-jp-dir")
	fs.StringVar(&agentsJPDir, "agents-jp-dir", "", "directory containing source agents (default: <cwd>/agents-jp); mutually exclusive with --skills-dir/--skills-jp-dir")

	if err := fs.Parse(args); err != nil {
		return Config{}, nil, fmt.Errorf("parse flags: %w", err)
	}

	skillsFlagSet := skillsDir != "" || skillsJPDir != ""
	agentsFlagSet := agentsDir != "" || agentsJPDir != ""
	if skillsFlagSet && agentsFlagSet {
		return Config{}, nil, fmt.Errorf("--skills-dir/--skills-jp-dir cannot be combined with --agents-dir/--agents-jp-dir")
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
	if agentsFlagSet {
		cfg.Mode = ModeAgents
		if cfg.AgentsDir, err = resolved(agentsDir, defaultAgentsDirName); err != nil {
			return Config{}, nil, err
		}
		if cfg.AgentsJPDir, err = resolved(agentsJPDir, defaultAgentsJPDirName); err != nil {
			return Config{}, nil, err
		}
		return cfg, fs.Args(), nil
	}

	cfg.Mode = ModeSkills
	if cfg.SkillsDir, err = resolved(skillsDir, defaultSkillsDirName); err != nil {
		return Config{}, nil, err
	}
	if cfg.SkillsJPDir, err = resolved(skillsJPDir, defaultSkillsJPDirName); err != nil {
		return Config{}, nil, err
	}

	return cfg, fs.Args(), nil
}
