package cli

import (
	"context"
	"os"
	"path/filepath"
)

// agentsMetaDirName is the hidden directory under agents/ that holds one TM
// JSON file per agent, mirroring skills/<name>/.translate-meta without
// requiring a per-agent directory (agent targets are single files).
const agentsMetaDirName = ".translate-meta"

// JPRoot returns the directory that a target's Japanese-source relative
// paths are resolved against: skills-jp/<name>/ for a skill (many files),
// or agents-jp/ itself for an agent (whose only relevant file is
// <name>.md, a single entry directly under this root).
func (cfg Config) JPRoot(name string) string {
	if cfg.Mode == ModeAgents {
		return cfg.AgentsJPDir
	}
	return filepath.Join(cfg.SkillsJPDir, name)
}

// ENRoot returns the directory that a target's English-side relative paths
// are resolved against, mirroring JPRoot.
func (cfg Config) ENRoot(name string) string {
	if cfg.Mode == ModeAgents {
		return cfg.AgentsDir
	}
	return filepath.Join(cfg.SkillsDir, name)
}

// MetaPath returns the translation-memory file for the named target:
// skills/<name>/.translate-meta for a skill, or
// agents/.translate-meta/<name>.json for an agent (agents share a single
// flat directory, so their TM entries cannot live alongside the file the
// way a skill's per-skill directory allows).
func (cfg Config) MetaPath(name string) string {
	if cfg.Mode == ModeAgents {
		return filepath.Join(cfg.AgentsDir, agentsMetaDirName, name+".json")
	}
	return filepath.Join(cfg.SkillsDir, name, metaFileName)
}

// ListFiles enumerates the files that make up the named target: every file
// under skills-jp/<name>/ for a skill, or the single file <name>.md under
// agents-jp/ for an agent. Both modes classify entries the same way, so
// translate/copy/preserve dispatch is identical downstream.
func (cfg Config) ListFiles(name string) ([]fileEntry, error) {
	if cfg.Mode == ModeAgents {
		relPath := name + ".md"
		entry, err := classifyPolicy(relPath, filepath.Join(cfg.AgentsJPDir, relPath))
		if err != nil {
			return nil, err
		}
		return []fileEntry{entry}, nil
	}
	return listManifestFiles(cfg.JPRoot(name))
}

// Orphans reports stale English-side files for the named target. In skills
// mode this is every file under skills/<name>/ with no skills-jp/<name>/
// counterpart. Agent mode uses different semantics: see agentOrphans.
func (cfg Config) Orphans(name string) ([]string, error) {
	if cfg.Mode == ModeAgents {
		return agentOrphans(cfg, name)
	}
	return listOrphans(cfg.SkillsDir, cfg.SkillsJPDir, name)
}

// agentOrphans flags agents/<name>.md as an orphan only when this agent has
// a translation-memory record (agents/.translate-meta/<name>.json exists)
// but its Japanese source at agents-jp/<name>.md has since been deleted.
// Unlike skills mode, an EN agent file with no JP counterpart is never
// itself an orphan candidate: agents/ legitimately holds English-only
// agents that were never translation targets, and every such agent would
// otherwise be misreported.
func agentOrphans(cfg Config, name string) ([]string, error) {
	metaPath := cfg.MetaPath(name)
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		return []string{}, nil
	} else if err != nil {
		return nil, err
	}

	jpPath := filepath.Join(cfg.AgentsJPDir, name+".md")
	if _, err := os.Stat(jpPath); os.IsNotExist(err) {
		return []string{name + ".md"}, nil
	} else if err != nil {
		return nil, err
	}
	return []string{}, nil
}

// legacyHash computes the comparison hash used to evaluate a target's
// legacy (pre-v2, bare-hex) meta content: a whole-directory hash for a
// skill, or a whole-file hash for an agent's single source file. No agent
// meta file in that format is ever written, so the agent branch only covers
// a hand-written one; it exists so resolveMetaState stays total rather than
// erroring on a case it cannot classify.
func legacyHash(ctx context.Context, cfg Config, name string) (string, error) {
	if cfg.Mode == ModeAgents {
		content, err := os.ReadFile(filepath.Join(cfg.AgentsJPDir, name+".md"))
		if err != nil {
			return "", err
		}
		return hashSegment(string(content)), nil
	}
	return hashDir(ctx, cfg.JPRoot(name))
}
