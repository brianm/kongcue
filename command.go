package kongcue

import (
	"fmt"
	"io"
	"os"

	"cuelang.org/go/cue/format"
	"github.com/alecthomas/kong"
)

// ConfigDoc is a Kong command that outputs the CUE schema for the CLI.
// Embed this in your CLI struct to add a command that prints the config schema.
//
// The generated schema uses named CUE definitions (#Root, #CommandName, etc.)
// and can be used to validate configuration files.
//
// Usage:
//
//	type cli struct {
//	    Agent     agentCmd          `cmd:""`
//	    ConfigDoc kongcue.ConfigDoc `cmd:"config-doc" help:"Print CUE schema for config file"`
//	}
//
// Running `./myapp config-doc` outputs the schema to stdout.
type ConfigDoc struct {
	// Output is the writer for schema output. Defaults to os.Stdout.
	// Exposed for testing.
	Output io.Writer `kong:"-"`
}

// BeforeApply is called by Kong before validation. Using BeforeApply (instead of
// AfterApply) allows this command to run without requiring other flags to be set,
// similar to --help.
func (c *ConfigDoc) BeforeApply(app *kong.Kong, schemaOpts *SchemaOptions) error {
	// Get schema options (set via AllowUnknownFields)
	opts := schemaOpts.toInternal()

	file := GenerateSchemaFile(app.Model, opts)

	src, err := format.Node(file)
	if err != nil {
		return fmt.Errorf("failed to format schema: %w", err)
	}

	out := c.Output
	if out == nil {
		out = os.Stdout
	}

	_, err = out.Write(src)
	if err != nil {
		return err
	}

	// Exit cleanly without running validation or the command.
	// If Output is set (testing), don't exit so tests can check the output.
	if c.Output == nil {
		os.Exit(0)
	}
	return nil
}
