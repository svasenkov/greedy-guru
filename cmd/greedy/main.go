package main

import (
	"os"

	"greedy.guru/greedy/internal/cli"
)

func main() {
	os.Exit(cli.Dispatch(os.Args[1:], os.Stdout, os.Stderr))
}
