package main

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/brianm/kongcue"
)

type cli struct {
	Name   string         `default:"world"`
	Config kongcue.Config `default:"./example.{yml,json,cue}" sep:";"`
}

func (c *cli) Run() error {
	fmt.Printf("Hello, %s\n", c.Name)
	return nil
}

func main() {

	//logger := slog.Default()

	var c cli
	ktx := kong.Parse(&c)
	ktx.FatalIfErrorf(ktx.Run())
}
