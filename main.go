package main

import (
	"context"
	"encoding/json"
	"fmt"
	"lam.local/lam2/internal/app"
	"lam.local/lam2/internal/cli"
	"lam.local/lam2/internal/commands"
	"lam.local/lam2/internal/desktop"
	"lam.local/lam2/internal/fault"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if e := run(); e != nil {
		_ = json.NewEncoder(os.Stderr).Encode(map[string]any{"error": fault.Public(e)})
		os.Exit(1)
	}
}
func run() error {
	v, e := commands.Parse(os.Args[1:])
	if e != nil {
		return e
	}
	if v.Mode == "help" {
		_, e = fmt.Fprint(os.Stdout, commands.Help)
		return e
	}
	core, e := app.Open(v.Home, nil)
	if e != nil {
		return e
	}
	defer core.Close()
	if v.Mode == "desktop" {
		return desktop.Run(core)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return cli.Execute(ctx, core, v, os.Stdin, os.Stdout, os.Stderr)
}
