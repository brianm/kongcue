package main

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/brianm/kongcue"
)

type cli struct {
	Name     string         `default:"world"`
	Required bool           `required:""`
	Config   kongcue.Config `default:"./example.{yml,json,cue}" sep:";"`
}

func (c *cli) Run() error {
	fmt.Printf("Hello, %s\n", c.Name)
	return nil
}

func main() {
	var c cli
	ktx := kong.Parse(&c)
	ktx.FatalIfErrorf(ktx.Run())
}
