package main

import (
	"fmt"
	"os"
	"strings"
)

// Version is overridden at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		runServe(nil)
		return
	}

	switch args[0] {
	case "serve":
		runServe(args[1:])
	case "version", "-version", "--version":
		fmt.Fprintf(os.Stdout, "azemu %s\n", Version)
	case "help", "-h", "-help", "--help":
		printUsage(os.Stderr)
	default:
		if strings.HasPrefix(args[0], "-") {
			runServe(args)
			return
		}
		fmt.Fprintf(os.Stderr, "azemu: unknown command %q\n\n", args[0])
		printUsage(os.Stderr)
		os.Exit(1)
	}
}

func printUsage(w *os.File) {
	fmt.Fprintf(w, "azemu %s -- local Azure emulator for Terraform\n\n", Version)
	fmt.Fprintf(w, "Usage: azemu <command> [flags]\n\n")
	fmt.Fprintf(w, "Commands:\n")
	fmt.Fprintf(w, "  serve       Start the emulator (default when no command given)\n")
	fmt.Fprintf(w, "  version     Print version and exit\n")
	fmt.Fprintf(w, "  help        Print this help and exit\n\n")
	fmt.Fprintf(w, "Run 'azemu <command> -help' for command-specific flags.\n\n")
}
