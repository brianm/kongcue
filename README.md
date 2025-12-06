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
	ktx := kong.Parse(&c)
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
