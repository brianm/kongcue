package main

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/brianm/kongcue"
)

type GreetCmd struct {
	Excited int `default:"0" help:"How excited are you to see this person?"`
}

type DepartCmd struct {
	Sadness int `help:"How sad are you to be leaving?" required:""`
}

type cli struct {
	Name      string            `default:"world" help:"The name of the person to greet"`
	Stuff     bool              `required:"" help:"A required flag, about stuff"`
	Config    kongcue.Config    `default:"./example.{yml,json,cue}" sep:";" help:"Config file paths"`
	ConfigDoc kongcue.ConfigDoc `cmd:"" help:"Print config schema"`
	Greet     GreetCmd          `cmd:"" help:"Issue a greeting"`
	Depart    DepartCmd         `cmd:"" help:"Issue a valediction"`

	message string
}

func (g GreetCmd) Run(c *cli) error {
	var message string
	switch {
	case g.Excited > 10:
		message = "WOW, HELLO %s, OMG OMG, I AM SO EXCITED TO MEET YOU!!!!!1!\n"
	case g.Excited >= 5:
		message = "Hello, %s, it's exciting to see you!\n"
	default:
		message = "Hello, %s\n"
	}

	fmt.Printf(message, c.Name)
	return nil
}

func (g DepartCmd) Run(c *cli) error {
	var message string
	switch {
	case g.Sadness >= 5:
		message = "Bye %s, I cannot wait to see you again soon!\n"
	case g.Sadness >= 5:
		message = "Bye %s, I'll miss you.\n"
	default:
		message = "Bye, %s\n"
	}

	fmt.Printf(message, c.Name)
	return nil
}

func main() {
	var c cli
	ktx := kong.Parse(&c, kongcue.AllowUnknownFields("messy"))
	ktx.FatalIfErrorf(ktx.Run(c))
}
