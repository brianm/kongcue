package kongcue

import (
	"fmt"
	"reflect"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/token"
	"github.com/alecthomas/kong"
)

// schemaOptions is the internal options struct used during schema generation.
type schemaOptions struct {
	allowUnknownPaths []string // Paths where unknown fields are allowed (empty = nowhere, nil with allowAll = everywhere)
	allowAll          bool     // Allow unknown fields everywhere (backwards compat for no-arg call)
	permissiveTypes   bool     // Use _ for all types (for unknown field checking only)
}

// pendingOpts accumulates schema options set during kong.New().
// Reset when getSchemaOptions() is called during BeforeResolve.
var pendingOpts = &schemaOptions{}

// getSchemaOptions returns the current options and resets for next parse.
func getSchemaOptions() *schemaOptions {
	opts := pendingOpts
	pendingOpts = &schemaOptions{} // Reset for next kong.New()
	return opts
}

// AllowUnknownFields returns a Kong option that allows unknown config keys.
// With no arguments, unknown fields are allowed everywhere (backwards compatible).
// With path arguments, unknown fields are only allowed at those paths and their descendants.
//
// Paths use dot notation matching the config structure (e.g., "server", "server.tls").
//
// Usage:
//
//	kong.Parse(&cli, kongcue.AllowUnknownFields())                    // allow everywhere
//	kong.Parse(&cli, kongcue.AllowUnknownFields("extra", "legacy"))   // allow at specific paths
func AllowUnknownFields(paths ...string) kong.Option {
	return kong.OptionFunc(func(k *kong.Kong) error {
		if len(paths) == 0 {
			pendingOpts.allowAll = true
		} else {
			pendingOpts.allowUnknownPaths = append(pendingOpts.allowUnknownPaths, paths...)
		}
		return nil
	})
}

// shouldAllowUnknown checks if unknown fields should be allowed at the given path.
// Returns true if:
// - allowAll is set (no-arg AllowUnknownFields())
// - path matches one of allowUnknownPaths exactly
// - path is a descendant of one of allowUnknownPaths
func (opts *schemaOptions) shouldAllowUnknown(path string) bool {
	if opts.allowAll {
		return true
	}
	for _, allowed := range opts.allowUnknownPaths {
		if path == allowed {
			return true
		}
		// Check if path is under allowed (allowed is a prefix)
		if strings.HasPrefix(path, allowed+".") {
			return true
		}
	}
	return false
}

// GenerateSchema creates a CUE schema from a Kong application model.
// The schema uses closed structs to reject unknown config keys unless
// allowUnknownFields is set in options.
func GenerateSchema(ctx *cue.Context, app *kong.Application, opts *schemaOptions) (cue.Value, error) {
	if opts == nil {
		opts = &schemaOptions{}
	}

	// Build AST from Kong model (root path is empty)
	rootStruct := buildNodeSchema(app.Node, "", opts)

	// Wrap in close() unless unknown fields are allowed at root
	var expr ast.Expr = rootStruct
	if !opts.shouldAllowUnknown("") {
		expr = wrapInClose(rootStruct)
	}

	// Format AST to source
	src, err := format.Node(expr)
	if err != nil {
		return cue.Value{}, fmt.Errorf("failed to format schema: %w", err)
	}

	// Compile to CUE value
	schemaVal := ctx.CompileBytes(src, cue.Filename("generated-schema"))
	if err := schemaVal.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("failed to compile schema: %w", err)
	}

	return schemaVal, nil
}

// ValidateConfig validates that config conforms to the schema.
// Returns an error if config contains unknown fields or type mismatches.
func ValidateConfig(schema cue.Value, config cue.Value) error {
	// Unify schema with config - this will produce errors for:
	// 1. Unknown fields (due to close())
	// 2. Type mismatches
	unified := schema.Unify(config)

	if err := unified.Err(); err != nil {
		return formatValidationError(err)
	}

	// Validate to catch additional constraint violations
	if err := unified.Validate(); err != nil {
		return formatValidationError(err)
	}

	return nil
}

// buildNodeSchema recursively builds a CUE struct AST from a Kong node.
// path is the dot-separated path to this node (empty string for root).
func buildNodeSchema(node *kong.Node, path string, opts *schemaOptions) *ast.StructLit {
	var fields []any
	existingFields := make(map[string]bool)

	// Add flags from this node
	for _, flag := range node.Flags {
		// Skip hidden flags, help, and config flags
		if flag.Hidden || flag.Name == "config" || flag.Name == "help" || flag.Name == "help-all" {
			continue
		}

		fieldName := kebabToSnake(flag.Name)
		existingFields[fieldName] = true
		var fieldType ast.Expr
		if opts.permissiveTypes {
			fieldType = ast.NewIdent("_")
		} else {
			fieldType = valueToType(flag.Value)
		}

		// Create optional field: fieldName?: type
		field := &ast.Field{
			Label:      ast.NewIdent(fieldName),
			Constraint: token.OPTION,
			Value:      fieldType,
		}
		fields = append(fields, field)
	}

	// Add child commands as nested schemas
	for _, child := range node.Children {
		if child.Type != kong.CommandNode {
			continue
		}

		existingFields[child.Name] = true

		// Build child path
		childPath := child.Name
		if path != "" {
			childPath = path + "." + child.Name
		}

		childSchema := buildNodeSchema(child, childPath, opts)
		var childExpr ast.Expr = childSchema
		if !opts.shouldAllowUnknown(childPath) {
			childExpr = wrapInClose(childSchema)
		}

		field := &ast.Field{
			Label:      ast.NewIdent(child.Name),
			Constraint: token.OPTION,
			Value:      childExpr,
		}
		fields = append(fields, field)
	}

	// Add allowed paths that don't exist as commands/flags
	// e.g., AllowUnknownFields("messy") adds messy?: _ at root
	for _, allowed := range opts.allowUnknownPaths {
		fieldName := opts.getAllowedFieldAtPath(allowed, path)
		if fieldName != "" && !existingFields[fieldName] {
			existingFields[fieldName] = true
			field := &ast.Field{
				Label:      ast.NewIdent(fieldName),
				Constraint: token.OPTION,
				Value:      ast.NewIdent("_"), // Open - allows anything
			}
			fields = append(fields, field)
		}
	}

	return &ast.StructLit{Elts: toDecls(fields)}
}

// getAllowedFieldAtPath returns the field name to add at the given path
// for an allowed path. Returns empty string if the allowed path doesn't
// apply at this level.
// e.g., allowed="messy", path="" -> "messy"
// e.g., allowed="foo.bar", path="" -> "foo"
// e.g., allowed="foo.bar", path="foo" -> "bar"
// e.g., allowed="messy", path="foo" -> ""
func (opts *schemaOptions) getAllowedFieldAtPath(allowed, path string) string {
	if path == "" {
		// At root, extract first component
		if idx := strings.Index(allowed, "."); idx != -1 {
			return allowed[:idx]
		}
		return allowed
	}

	// Check if allowed starts with path
	prefix := path + "."
	if !strings.HasPrefix(allowed, prefix) {
		return ""
	}

	// Extract next component after path
	rest := allowed[len(prefix):]
	if idx := strings.Index(rest, "."); idx != -1 {
		return rest[:idx]
	}
	return rest
}

// valueToType converts a Kong value to a CUE type expression.
// Types are made coercible by allowing string alternatives.
func valueToType(v *kong.Value) ast.Expr {
	// Handle slices
	if v.IsSlice() {
		elemType := sliceElemType(v.Target)
		return &ast.ListLit{
			Elts: []ast.Expr{&ast.Ellipsis{Type: elemType}},
		}
	}

	// Handle maps
	if v.IsMap() {
		// Open struct pattern: {[string]: _}
		return ast.NewStruct()
	}

	// Handle counters (like -v -v -v for verbosity)
	if v.IsCounter() {
		return ast.NewIdent("int")
	}

	// Handle booleans
	if v.IsBool() {
		return ast.NewIdent("bool")
	}

	// Use reflection for other types
	return kindToType(v.Target.Kind())
}

// kindToType converts a reflect.Kind to a CUE type expression.
func kindToType(k reflect.Kind) ast.Expr {
	switch k {
	case reflect.String:
		return ast.NewIdent("string")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return ast.NewIdent("int")
	case reflect.Float32, reflect.Float64:
		return ast.NewIdent("number")
	case reflect.Bool:
		return ast.NewIdent("bool")
	default:
		return ast.NewIdent("_")
	}
}

// sliceElemType returns the CUE type for slice elements.
func sliceElemType(v reflect.Value) ast.Expr {
	if v.Kind() != reflect.Slice {
		return ast.NewIdent("_")
	}
	elemKind := v.Type().Elem().Kind()
	return kindToType(elemKind)
}

// wrapInClose wraps a struct in close() to reject unknown fields.
func wrapInClose(s *ast.StructLit) ast.Expr {
	return &ast.CallExpr{
		Fun:  ast.NewIdent("close"),
		Args: []ast.Expr{s},
	}
}

// kebabToSnake converts kebab-case to snake_case.
func kebabToSnake(s string) string {
	return strings.ReplaceAll(s, "-", "_")
}

// toDecls converts a slice of fields to ast.Decl slice.
func toDecls(fields []any) []ast.Decl {
	decls := make([]ast.Decl, len(fields))
	for i, f := range fields {
		decls[i] = f.(ast.Decl)
	}
	return decls
}

// formatValidationError formats CUE validation errors for user display.
func formatValidationError(err error) error {
	errStr := err.Error()

	// Check for "field not allowed" pattern (unknown fields)
	if strings.Contains(errStr, "field not allowed") {
		return fmt.Errorf("unknown configuration key: %w\n"+
			"Hint: Check that all config keys correspond to valid CLI flags", err)
	}

	return fmt.Errorf("configuration validation failed: %w", err)
}
