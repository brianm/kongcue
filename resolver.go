package kongcue

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/errors"
	"github.com/alecthomas/kong"
)

// cueResolver implements kong.Resolver using direct CUE value lookups
type cueResolver struct {
	value cue.Value
}

// NewResolver creates a Kong resolver backed by a CUE value.
// The resolver automatically maps CLI flag names (kebab-case) to config keys (snake_case).
//
// Config paths are built from the command hierarchy. For example, a flag "--ca-url"
// on command "agent" will look up "agent.ca_url" in the CUE value.
// Global flags (not under a subcommand) use just the flag name.
//
// Example:
//
//	config, _ := LoadAndUnifyPaths([]string{"~/.myapp/*.yaml"})
//	ctx := kong.Parse(&cli, kong.Resolvers(NewResolver(config)))
func NewResolver(value cue.Value) kong.Resolver {
	return &cueResolver{value: value}
}

type Config []string

func (r Config) BeforeResolve(k *kong.Kong, ctx *kong.Context, trace *kong.Path, schemaOpts *SchemaOptions) error {
	paths := []string(ctx.FlagValue(trace.Flag).(Config))
	expanded := make([]string, len(paths))
	for i, path := range paths {
		expanded[i] = kong.ExpandPath(path)
	}
	val, err := LoadAndUnifyPaths(expanded)
	if err != nil {
		return fmt.Errorf("unable to load config: %w", err)
	}

	// Check if any config was actually loaded (has fields)
	// If no config files were found, skip validation and let Kong handle CLI flags
	iter, _ := val.Fields()
	hasConfig := iter.Next()

	if !hasConfig {
		// No config loaded - just set up empty resolver, let Kong handle validation
		ctx.AddResolver(&cueResolver{value: val})
		return nil
	}

	// Generate schema and validate config early to report config errors clearly
	opts := schemaOpts.toInternal()
	schema, err := GenerateSchema(val.Context(), k.Model, opts)
	if err != nil {
		return fmt.Errorf("failed to generate config schema: %w", err)
	}

	var allErrs errors.Error

	// First pass: check for unknown fields using permissive types
	// Skip if allowAll is set (unknown fields allowed everywhere)
	if !opts.allowAll {
		permissiveOpts := &schemaOptions{
			permissiveTypes:   true,
			allowUnknownPaths: opts.allowUnknownPaths, // Inherit path-specific allows
			allowAll:          opts.allowAll,
		}
		permissiveSchema, err := GenerateSchema(val.Context(), k.Model, permissiveOpts)
		if err != nil {
			return fmt.Errorf("failed to generate config schema: %w", err)
		}
		if err := val.Unify(permissiveSchema).Validate(); err != nil {
			allErrs = errors.Append(allErrs, errors.Promote(err, ""))
		}
	}

	// Second pass: check types with strict schema
	// Use Concrete(true) to ensure required fields are present
	merged := val.Unify(schema)
	if err := merged.Validate(cue.Concrete(true)); err != nil {
		allErrs = errors.Append(allErrs, errors.Promote(err, ""))
	}

	if allErrs != nil {
		return errors.New(filterErrorDetails(allErrs))
	}

	ctx.Bind(merged)
	ctx.AddResolver(&cueResolver{value: merged})
	return nil
}

func (r *cueResolver) Validate(app *kong.Application) error {
	// Schema validation already done in BeforeResolve
	return nil
}

// filterErrorDetails formats CUE errors, removing references to generated files
// and adding helpful context for common error types.
func filterErrorDetails(err error) string {
	details := errors.Details(err, nil)
	// Filter out lines referencing the generated schema
	var filtered []string
	hasIncomplete := false
	hasNotAllowed := false
	for line := range strings.SplitSeq(details, "\n") {
		if strings.Contains(line, "generated-schema") {
			continue
		}
		if strings.Contains(line, "incomplete value") {
			hasIncomplete = true
		}
		if strings.Contains(line, "field not allowed") {
			hasNotAllowed = true
		}
		filtered = append(filtered, line)
	}
	result := strings.Join(filtered, "\n")

	// Add helpful hints based on error type
	if hasIncomplete {
		result += "\nHint: Required fields must be provided in the config file"
	}
	if hasNotAllowed {
		result += "\nHint: Check that all config keys correspond to valid CLI flags"
	}
	return result
}

func (r *cueResolver) Resolve(ctx *kong.Context, parent *kong.Path, flag *kong.Flag) (any, error) {
	// Build the config path from command context
	cmdPath := getCommandPath(parent)

	// Normalize flag name: convert kebab-case to snake_case
	flagName := strings.ReplaceAll(flag.Name, "-", "_")

	// Build full path: e.g., "agent.ca_url" or just "insecure" for globals
	var cuePath string
	if len(cmdPath) == 0 {
		cuePath = flagName
	} else {
		cuePath = strings.Join(append(cmdPath, flagName), ".")
	}

	// Look up the value in CUE
	val := r.value.LookupPath(cue.ParsePath(cuePath))
	if !val.Exists() {
		return nil, nil
	}

	// Extract the value based on type
	return extractValue(val, flag.IsSlice())
}

// getCommandPath extracts the command path from kong's parent path
func getCommandPath(parent *kong.Path) []string {
	if parent == nil {
		return nil
	}

	var path []string
	for n := parent.Node(); n != nil; n = n.Parent {
		if n.Type == kong.CommandNode && n.Name != "" {
			path = append([]string{n.Name}, path...)
		}
	}
	return path
}

// extractValue extracts a Go value from a CUE value
func extractValue(val cue.Value, isSlice bool) (any, error) {
	// Handle lists/slices
	if isSlice {
		iter, err := val.List()
		if err != nil {
			// Not a list, try as single value
			str, err := val.String()
			if err != nil {
				return nil, nil
			}
			return str, nil
		}

		var items []string
		for iter.Next() {
			str, err := iter.Value().String()
			if err != nil {
				continue
			}
			items = append(items, str)
		}

		if len(items) == 0 {
			return nil, nil
		}
		if len(items) == 1 {
			return items[0], nil
		}
		return strings.Join(items, ","), nil
	}

	// Handle booleans
	if b, err := val.Bool(); err == nil {
		if b {
			return true, nil
		}
		return nil, nil // Don't return false, let kong use default
	}

	// Handle integers - return as int (not int64) for Kong compatibility
	if i, err := val.Int64(); err == nil {
		if i > 0 {
			return int(i), nil
		}
		return nil, nil // Don't return 0, let kong use default
	}

	// Handle strings
	if str, err := val.String(); err == nil {
		if str != "" {
			return str, nil
		}
		return nil, nil
	}

	return nil, nil
}

// Ensure cueResolver implements kong.Resolver
var _ kong.Resolver = (*cueResolver)(nil)
