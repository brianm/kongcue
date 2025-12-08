package kongcue

import (
	"fmt"
	"reflect"
	"sort"
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

// SchemaOptions holds configuration for schema generation.
// Exported so it can be received via Kong's dependency injection in hooks.
type SchemaOptions struct {
	AllowUnknownPaths []string
	AllowAll          bool
}

// toInternal converts exported SchemaOptions to internal schemaOptions.
func (o *SchemaOptions) toInternal() *schemaOptions {
	if o == nil {
		return &schemaOptions{}
	}
	return &schemaOptions{
		allowUnknownPaths: o.AllowUnknownPaths,
		allowAll:          o.AllowAll,
	}
}

// defaultSchemaOptions is the default binding used when AllowUnknownFields is not called.
var defaultSchemaOptions = &SchemaOptions{}

// Options returns a Kong option that sets up kongcue's schema validation.
// This must be included when using kongcue.Config or kongcue.ConfigDoc.
// If you want to allow unknown fields, use AllowUnknownFields() instead.
//
// Usage:
//
//	kong.Parse(&cli, kongcue.Options())
func Options() kong.Option {
	return kong.Bind(defaultSchemaOptions)
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
	opts := &SchemaOptions{}
	if len(paths) == 0 {
		opts.AllowAll = true
	} else {
		opts.AllowUnknownPaths = paths
	}
	// Bind options so hooks can receive them via DI
	return kong.Bind(opts)
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
// The schema uses named definitions (#Root, #CommandName) and closed structs
// to reject unknown config keys unless allowUnknownFields is set in options.
// Returns the #Root definition as a cue.Value for validation.
func GenerateSchema(ctx *cue.Context, app *kong.Application, opts *schemaOptions) (cue.Value, error) {
	if opts == nil {
		opts = &schemaOptions{}
	}

	// Generate schema with named definitions
	file := GenerateSchemaFile(app, opts)

	// Format AST to source
	src, err := format.Node(file)
	if err != nil {
		return cue.Value{}, fmt.Errorf("failed to format schema: %w", err)
	}

	// Compile to CUE value
	schemaVal := ctx.CompileBytes(src, cue.Filename("generated-schema"))
	if err := schemaVal.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("failed to compile schema: %w", err)
	}

	// Return the #Root definition for validation
	root := schemaVal.LookupPath(cue.ParsePath("#Root"))
	if err := root.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("failed to lookup #Root: %w", err)
	}

	return root, nil
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

// addDocComment adds a documentation comment to a CUE AST field.
func addDocComment(field *ast.Field, text string) {
	comment := &ast.Comment{Text: "// " + text}
	cg := &ast.CommentGroup{List: []*ast.Comment{comment}}
	ast.AddComment(field, cg)
}

// addHeaderComment adds an explanatory header comment to the schema.
func addHeaderComment(field *ast.Field) {
	cg := &ast.CommentGroup{
		List: []*ast.Comment{
			{Text: "// Configuration schema for validating config files."},
			{Text: "//"},
			{Text: "// This schema is written in CUE, a configuration language that"},
			{Text: "// validates and defines data. Learn more at https://cuelang.org"},
			{Text: "//"},
			{Text: "// To validate your config file against this schema:"},
			{Text: "//   1. Save this schema to a file (e.g., schema.cue)"},
			{Text: "//   2. Run: cue vet -d '#Root' schema.cue your-config.yaml"},
			{Text: "//"},
			{Text: "// Fields marked with ? are optional. Fields without ? are required."},
		},
	}
	ast.AddComment(field, cg)
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

	// Check for "incomplete value" pattern (missing required fields)
	if strings.Contains(errStr, "incomplete value") {
		return fmt.Errorf("missing required configuration field: %w", err)
	}

	return fmt.Errorf("configuration validation failed: %w", err)
}

// GenerateSchemaWithDefinitions creates a CUE file with named definitions.
// Each command becomes a separate definition (e.g., #Agent, #Server).
// The root schema references these definitions.
//
// Example output:
//
//	#Root: close({
//	    verbose?: int
//	    agent?: #Agent
//	})
//	#Agent: close({
//	    ca_url?: string
//	})
// commandDef holds a command's schema definition and its config path.
type commandDef struct {
	structLit *ast.StructLit
	path      string // dot-separated path for AllowUnknownFields lookup
}

// GenerateSchemaFile creates a CUE AST file with named definitions.
// Each command becomes a separate definition (e.g., #Agent, #Server).
// The root schema is in #Root and references command definitions.
//
// Example output:
//
//	#Root: close({
//	    verbose?: int
//	    agent?: #Agent
//	})
//	#Agent: close({
//	    ca_url?: string
//	})
func GenerateSchemaFile(app *kong.Application, opts *schemaOptions) *ast.File {
	if opts == nil {
		opts = &schemaOptions{}
	}

	file := &ast.File{}
	definitions := make(map[string]commandDef)

	// Collect all command definitions first
	collectCommandDefinitions(app.Node, nil, definitions, opts)

	// Build root definition with references to command definitions
	rootStruct := buildRootWithReferences(app.Node, definitions, opts)

	// When unknown fields are allowed, add "..." to make it an open struct.
	// CUE definitions are closed by default, so we need explicit "..." for openness.
	// When not allowed, wrap in close() for explicit rejection.
	var rootExpr ast.Expr = rootStruct
	if opts.shouldAllowUnknown("") {
		// Add ellipsis to make struct open
		rootStruct.Elts = append(rootStruct.Elts, &ast.Ellipsis{})
	} else {
		rootExpr = wrapInClose(rootStruct)
	}

	// Add root definition first, with header comment explaining CUE
	rootField := &ast.Field{
		Label: ast.NewIdent("#Root"),
		Value: rootExpr,
	}
	addHeaderComment(rootField)
	file.Decls = append(file.Decls, rootField)

	// Add command definitions in sorted order for deterministic output
	for _, defName := range sortedKeys(definitions) {
		def := definitions[defName]

		var defExpr ast.Expr = def.structLit
		if opts.shouldAllowUnknown(def.path) {
			// Add ellipsis to make struct open (CUE definitions are closed by default)
			def.structLit.Elts = append(def.structLit.Elts, &ast.Ellipsis{})
		} else {
			defExpr = wrapInClose(def.structLit)
		}

		defField := &ast.Field{
			Label: ast.NewIdent("#" + defName),
			Value: defExpr,
		}
		file.Decls = append(file.Decls, defField)
	}

	return file
}

// collectCommandDefinitions recursively collects definitions for all commands.
// Each command node becomes a named definition with its flags as fields.
func collectCommandDefinitions(node *kong.Node, path []string, defs map[string]commandDef, opts *schemaOptions) {
	for _, child := range node.Children {
		if child.Type != kong.CommandNode {
			continue
		}

		// Skip ConfigDoc command as it's not a config option
		if isConfigDocCommand(child) {
			continue
		}

		childPath := append(path, child.Name)
		defName := commandDefName(childPath)
		dotPath := strings.Join(childPath, ".")

		// Build struct for this command's flags
		structLit := buildFlagsStruct(child, opts)

		// Add references to nested command definitions
		for _, grandchild := range child.Children {
			if grandchild.Type != kong.CommandNode {
				continue
			}

			// Skip ConfigDoc in nested commands too
			if isConfigDocCommand(grandchild) {
				continue
			}

			grandchildPath := append(childPath, grandchild.Name)
			grandchildDefName := commandDefName(grandchildPath)

			refField := &ast.Field{
				Label:      ast.NewIdent(grandchild.Name),
				Constraint: token.OPTION,
				Value:      ast.NewIdent("#" + grandchildDefName),
			}
			// Add command help text as comment
			if grandchild.Help != "" {
				addDocComment(refField, grandchild.Help)
			}
			structLit.Elts = append(structLit.Elts, refField)
		}

		// Add allowed paths that don't exist as commands/flags
		existingFields := make(map[string]bool)
		for _, f := range structLit.Elts {
			if field, ok := f.(*ast.Field); ok {
				if ident, ok := field.Label.(*ast.Ident); ok {
					existingFields[ident.Name] = true
				}
			}
		}
		for _, allowed := range opts.allowUnknownPaths {
			fieldName := opts.getAllowedFieldAtPath(allowed, dotPath)
			if fieldName != "" && !existingFields[fieldName] {
				existingFields[fieldName] = true
				field := &ast.Field{
					Label:      ast.NewIdent(fieldName),
					Constraint: token.OPTION,
					Value:      ast.NewIdent("_"),
				}
				structLit.Elts = append(structLit.Elts, field)
			}
		}

		defs[defName] = commandDef{structLit: structLit, path: dotPath}

		// Recursively collect nested commands
		collectCommandDefinitions(child, childPath, defs, opts)
	}
}

// buildFlagsStruct creates a struct with only flag fields (no nested commands).
func buildFlagsStruct(node *kong.Node, opts *schemaOptions) *ast.StructLit {
	var fields []ast.Decl

	for _, flag := range node.Flags {
		if flag.Hidden || flag.Name == "config" || flag.Name == "help" || flag.Name == "help-all" {
			continue
		}

		fieldName := kebabToSnake(flag.Name)
		var fieldType ast.Expr
		if opts.permissiveTypes {
			fieldType = ast.NewIdent("_")
		} else {
			fieldType = valueToType(flag.Value)
		}

		field := &ast.Field{
			Label: ast.NewIdent(fieldName),
			Value: fieldType,
		}
		// Only mark as optional if not required
		if !flag.Required {
			field.Constraint = token.OPTION
		}
		// Add help text as comment
		if flag.Help != "" {
			addDocComment(field, flag.Help)
		}
		fields = append(fields, field)
	}

	return &ast.StructLit{Elts: fields}
}

// buildRootWithReferences creates the root struct with global flags
// and references to command definitions.
func buildRootWithReferences(node *kong.Node, defs map[string]commandDef, opts *schemaOptions) *ast.StructLit {
	var fields []ast.Decl
	existingFields := make(map[string]bool)

	// Add global flags
	for _, flag := range node.Flags {
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

		field := &ast.Field{
			Label: ast.NewIdent(fieldName),
			Value: fieldType,
		}
		// Only mark as optional if not required
		if !flag.Required {
			field.Constraint = token.OPTION
		}
		// Add help text as comment
		if flag.Help != "" {
			addDocComment(field, flag.Help)
		}
		fields = append(fields, field)
	}

	// Add references to command definitions
	for _, child := range node.Children {
		if child.Type != kong.CommandNode {
			continue
		}

		// Skip config-doc command (ConfigDoc) as it's not a config option
		if isConfigDocCommand(child) {
			continue
		}

		existingFields[child.Name] = true
		defName := commandDefName([]string{child.Name})

		refField := &ast.Field{
			Label:      ast.NewIdent(child.Name),
			Constraint: token.OPTION,
			Value:      ast.NewIdent("#" + defName),
		}
		// Add command help text as comment
		if child.Help != "" {
			addDocComment(refField, child.Help)
		}
		fields = append(fields, refField)
	}

	// Add allowed paths that don't exist as commands/flags at root level
	for _, allowed := range opts.allowUnknownPaths {
		fieldName := opts.getAllowedFieldAtPath(allowed, "")
		if fieldName != "" && !existingFields[fieldName] {
			existingFields[fieldName] = true
			field := &ast.Field{
				Label:      ast.NewIdent(fieldName),
				Constraint: token.OPTION,
				Value:      ast.NewIdent("_"),
			}
			fields = append(fields, field)
		}
	}

	return &ast.StructLit{Elts: fields}
}

// isConfigDocCommand checks if a node is the ConfigDoc command.
// We skip this in schema generation as it's not a config option.
func isConfigDocCommand(node *kong.Node) bool {
	// Check if this is a ConfigDoc command by looking at the target type
	if node.Target.IsValid() && node.Target.Type().String() == "kongcue.ConfigDoc" {
		return true
	}
	return false
}

// pascalCase converts snake_case or kebab-case to PascalCase.
// "agent" -> "Agent", "ca-url" -> "CaUrl", "my_cmd" -> "MyCmd"
func pascalCase(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")

	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, "")
}

// commandDefName converts a command path to a PascalCase definition name.
// ["server", "tls"] -> "ServerTls"
func commandDefName(path []string) string {
	return pascalCase(strings.Join(path, "_"))
}

// sortedKeys returns map keys in sorted order for deterministic output.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
