package main

import (
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Version is overridden at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Caller().Logger()

	args := os.Args[1:]

	if len(args) == 0 {
		if err := runServe(nil); err != nil {
			log.Fatal().Err(err).Msg("serve failed")
		}
		return
	}

	switch args[0] {
	case "serve":
		if err := runServe(args[1:]); err != nil {
			log.Fatal().Err(err).Msg("serve failed")
		}
	case "tf":
		if err := runTF(args[1:]); err != nil {
			log.Fatal().Err(err).Msg("tf failed")
		}
	case "pulumi":
		if err := runPulumi(args[1:]); err != nil {
			log.Fatal().Err(err).Msg("pulumi failed")
		}
	case "kubectl":
		if err := runKubectl(args[1:]); err != nil {
			log.Fatal().Err(err).Msg("kubectl failed")
		}
	case "python":
		if err := runPython(args[1:]); err != nil {
			log.Fatal().Err(err).Msg("python failed")
		}
	case "parity":
		if err := runParity(args[1:]); err != nil {
			log.Fatal().Err(err).Msg("parity failed")
		}
	case "status":
		if err := runStatus(args[1:]); err != nil {
			log.Fatal().Err(err).Msg("status failed")
		}
	case "snapshot":
		if err := runSnapshot(args[1:]); err != nil {
			log.Fatal().Err(err).Msg("snapshot failed")
		}
	case "--version", "-version", "version":
		fmt.Fprintf(os.Stdout, "azemu %s\n", Version)
	case "--help", "-help", "-h", "help":
		printUsage(os.Stderr)
	default:
		if args[0][0] == '-' {
			if err := runServe(args); err != nil {
				log.Fatal().Err(err).Msg("serve failed")
			}
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
	fmt.Fprintf(w, "  serve       Start the emulator server (default)\n")
	fmt.Fprintf(w, "  tf          Run terraform with azemu env vars injected\n")
	fmt.Fprintf(w, "  pulumi      Run pulumi with azemu env vars injected\n")
	fmt.Fprintf(w, "  kubectl     Run kubectl with azemu env vars injected\n")
	fmt.Fprintf(w, "  python      Run python with Azure SDK env vars injected\n")
	fmt.Fprintf(w, "  parity      Show supported Azure resources\n")
	fmt.Fprintf(w, "  snapshot    Save, load, list, or reset state snapshots\n")
	fmt.Fprintf(w, "  status      Check if azemu is running\n")
	fmt.Fprintf(w, "  version     Print version and exit\n")
	fmt.Fprintf(w, "  help        Print this help and exit\n\n")
	fmt.Fprintf(w, "Run 'azemu <command> -help' for command-specific flags.\n\n")
	fmt.Fprintf(w, "When no command is given, azemu defaults to 'serve'.\n")
	fmt.Fprintf(w, "Legacy flag syntax (azemu --persist /tmp/state.json) is still supported.\n\n")
}
