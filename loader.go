// Package kongcue provides CUE-based configuration loading with Kong CLI integration.
// It supports YAML, JSON, and CUE file formats using CUE as the underlying parser,
// and provides a Kong resolver for seamless CLI flag and config file merging.
package kongcue

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/encoding/yaml"
	"github.com/bmatcuk/doublestar/v4"
)

// LoadAndUnifyPaths loads multiple config files and unifies them into a single CUE value.
// Supports glob patterns and mixed file types (.cue, .yaml, .yml, .json).
// Missing files are silently skipped. Returns error if files have conflicting values.
//
// The ~ character is expanded to the user's home directory.
func LoadAndUnifyPaths(patterns []string) (cue.Value, error) {
	ctx := cuecontext.New()
	var values []cue.Value
	var loadedPaths []string

	for _, pattern := range patterns {
		// Expand ~ to home directory
		expanded, err := expandPath(pattern)
		if err != nil {
			continue // Skip patterns with expansion errors
		}

		// Handle glob patterns
		matches, err := doublestar.FilepathGlob(expanded)
		if err != nil {
			continue // Invalid pattern, skip
		}
		if len(matches) == 0 {
			// Try as literal path (for non-glob patterns that don't exist)
			if _, err := os.Stat(expanded); err == nil {
				matches = []string{expanded}
			}
		}

		for _, path := range matches {
			val, err := loadSingleFile(ctx, path)
			if err != nil {
				return cue.Value{}, err
			}
			if !val.Exists() {
				continue // Skip unreadable files
			}

			values = append(values, val)
			loadedPaths = append(loadedPaths, path)
		}
	}

	if len(values) == 0 {
		// No config files found - return empty value (not an error)
		return ctx.CompileString("{}"), nil
	}

	// Unify all values
	result := values[0]
	for i, v := range values[1:] {
		result = result.Unify(v)
		if err := result.Err(); err != nil {
			return cue.Value{}, fmt.Errorf("config conflict between %s and %s: %w",
				loadedPaths[0], loadedPaths[i+1], err)
		}
	}

	return result, nil
}

// loadSingleFile loads a single config file, detecting type by extension.
// Returns empty value (not error) for unreadable files.
func loadSingleFile(ctx *cue.Context, path string) (cue.Value, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cue.Value{}, nil // Skip unreadable files
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".cue":
		// Use CUE's native parser for .cue files
		val := ctx.CompileBytes(data, cue.Filename(path))
		if err := val.Err(); err != nil {
			return cue.Value{}, fmt.Errorf("failed to parse %s: %w", path, err)
		}
		return val, nil
	case ".yaml", ".yml":
		file, err := yaml.Extract(path, data)
		if err != nil {
			return cue.Value{}, fmt.Errorf("failed to parse %s: %w", path, err)
		}
		val := ctx.BuildFile(file)
		if err := val.Err(); err != nil {
			return cue.Value{}, fmt.Errorf("failed to build %s: %w", path, err)
		}
		return val, nil
	case ".json":
		val := ctx.CompileBytes(data, cue.Filename(path))
		if err := val.Err(); err != nil {
			return cue.Value{}, fmt.Errorf("failed to parse %s: %w", path, err)
		}
		return val, nil
	default:
		// Try YAML as fallback
		file, err := yaml.Extract(path, data)
		if err != nil {
			return cue.Value{}, fmt.Errorf("failed to parse %s: %w", path, err)
		}
		val := ctx.BuildFile(file)
		if err := val.Err(); err != nil {
			return cue.Value{}, fmt.Errorf("failed to build %s: %w", path, err)
		}
		return val, nil
	}
}

// expandPath expands ~ to the user's home directory.
// Returns the path unchanged if it doesn't start with ~.
func expandPath(path string) (string, error) {
	if len(path) == 0 || path[0] != '~' {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	if len(path) == 1 {
		return home, nil
	}
	return filepath.Join(home, path[1:]), nil
}
