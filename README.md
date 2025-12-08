# kongcue

A Go library that bridges [Kong](https://github.com/alecthomas/kong) CLI parsing with [CUE](https://cuelang.org/)-based configuration files.

## Installation

```bash
go get github.com/brianm/kongcue
```

## Quick Start

Embed `kongcue.Config` in your CLI struct to automatically load config files and resolve flag values:

```go
package main

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/brianm/kongcue"
)

type cli struct {
	Name   string         `default:"world"`
	Config kongcue.Config `default:"./config.{yml,yaml,cue,json}" sep:";"`
}

func (c *cli) Run() error {
	fmt.Printf("Hello, %s\n", c.Name)
	return nil
}

func main() {
	var c cli
	ktx := kong.Parse(&c, kongcue.Options())
	err := ktx.Run()
	ktx.FatalIfErrorf(err)
}
```

With `config.yml`:
```yaml
name: "Brian"
```

```bash
./myapp              # Hello, Brian (from config)
./myapp --name Alice # Hello, Alice (CLI overrides config)
```

**Note**: `kongcue.Options()` is required when using `kongcue.Config` or `kongcue.ConfigDoc`. If you use `kongcue.AllowUnknownFields()`, it includes `Options()` automatically.

## Features

- **Multiple file formats**: YAML, JSON, and CUE files
- **Glob patterns**: Load configs with patterns like `~/.myapp/*.{yaml,yml}`
- **Config unification**: Multiple config files are merged; conflicts produce errors
- **Automatic name mapping**: CLI flags (`--ca-url`) map to config keys (`ca_url`)
- **Command hierarchy**: Flags resolve based on subcommand context
- **Schema validation**: Config keys are validated against CLI flags

## Glob Patterns

You can use bash style glob patterns (provided by [doublestar](https://github.com/bmatcuk/doublestar?tab=readme-ov-file#patterns)).

Note that if you use `{yml,yaml,cue,json}` style brace expansion and a default value, you will need to tell kong to use a seperator other than `,` or it will split inside the braces, so use something like:

```go
Config kongcue.Config `default:"./config.{yml,yaml,cue,json}" sep:";"`
````

This tells kong to use a `;` as the seperator between values, so you could do:

```go
Config kongcue.Config `default:"/etc/foo.cue;~/.config/foo/config.{yaml,cue,json}" sep:";"`
````

To tell it to look in `/etc/foo.cue` and `~/.config/foo/config.{yaml,cue,json}`.

## Schema Validation

By default, config files are validated against your CLI struct. Unknown keys that don't correspond to any CLI flag will cause an error:

```
error: unknown configuration key: typo_field: field not allowed
       Hint: Check that all config keys correspond to valid CLI flags
```

This catches typos and stale config keys early.

To allow extra fields in config files (useful if configs are shared with other tools), use `AllowUnknownFields()`:

```go
ctx := kong.Parse(&cli, kongcue.AllowUnknownFields())
```

## Schema Documentation Command

Add a command that prints the CUE schema for your CLI's configuration:

```go
type cli struct {
	Name      string            `help:"Name to greet"`
	Server    serverCmd         `cmd:"" help:"Run the server"`
	ConfigDoc kongcue.ConfigDoc `cmd:"config-doc" help:"Print config schema"`
	Config    kongcue.Config    `default:"./config.yaml"`
}
```

Running `./myapp config-doc` outputs a CUE schema:

```cue
// Configuration schema for validating config files.
//
// This schema is written in CUE, a configuration language that
// validates and defines data. Learn more at https://cuelang.org
//
// To validate your config file against this schema:
//   1. Save this schema to a file (e.g., schema.cue)
//   2. Run: cue vet -d '#Root' schema.cue your-config.yaml
//
// Fields marked with ? are optional. Fields without ? are required.
#Root: close({
	// Name to greet
	name?: string
	// Run the server
	server?: #Server
})
#Server: close({
	// Server port
	port?: int
})
```

The schema includes:
- **Help text as comments**: Kong `help:"..."` tags become CUE documentation
- **Required field markers**: Fields with `required:""` don't have `?` and must be present
- **Nested definitions**: Each subcommand gets its own `#Definition`

Users can validate their config files using the [CUE CLI](https://cuelang.org/docs/introduction/installation/):

```bash
./myapp config-doc > schema.cue
cue vet -d '#Root' schema.cue config.yaml
```

## Configuration Formats

All formats are parsed using CUE, which means you get CUE's type checking and unification:

**YAML** (`config.yaml`):
```yaml
verbose: 2
agent:
  ca_url: "https://ca.example.com"
```

**JSON** (`config.json`):
```json
{
  "verbose": 2,
  "agent": {
    "ca_url": "https://ca.example.com"
  }
}
```

**CUE** (`config.cue`):
```cue
verbose: 2
agent: {
  ca_url: "https://ca.example.com"
}
```

## Naming Convention

CLI flags use kebab-case, config files use snake_case:

| CLI Flag | Config Key |
|----------|------------|
| `--ca-url` | `ca_url` |
| `--log-file` | `log_file` |

This avoids quoted field names in CUE (where `-` is the subtraction operator).

## Command Path Resolution

Flags are resolved using the command hierarchy. For a CLI like:

```go
type cli struct {
    Verbose int `name:"verbose"`
    Agent struct {
        CaURL string `name:"ca-url"`
    } `cmd:""`
}
```

- `--verbose` resolves to `verbose` in config
- `agent --ca-url` resolves to `agent.ca_url` in config

## Multiple Config Files

Specify multiple config files with repeated flags:

```bash
./myapp --config base.yaml --config overrides.yaml
```

Files are unified in order. Conflicting values (same key, different values) produce an error.

## Low-Level API

For more control, use the loader and resolver directly:

```go
config, err := kongcue.LoadAndUnifyPaths([]string{
    "~/.myapp/config.yaml",
    "./local.yaml",
})
if err != nil {
    log.Fatal(err)
}

ctx := kong.Parse(&cli, kong.Resolvers(kongcue.NewResolver(config)))
```

## License

Apache-2.0
