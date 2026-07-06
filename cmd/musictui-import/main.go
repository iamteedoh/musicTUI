// Command musictui-import is a standalone CLI for transferring
// music libraries between streaming services. Complementary tool to
// the musicTUI terminal music player — runs entirely on the user's
// machine with their own OAuth client credentials.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/iamteedoh/musicTUI/internal/importcore/cliconfig"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var err error
	switch cmd {
	case "setup":
		err = runSetup(ctx, args)
	case "auth":
		err = runAuth(ctx, args)
	case "import":
		err = runImport(ctx, args)
	case "status":
		err = runStatus(ctx, args)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`musictui-import — transfer music libraries between streaming services

Usage:
  musictui-import <command> [flags]

Commands:
  setup            Paste your Google Cloud + Spotify OAuth client credentials
  auth <service>   Connect a service (service = youtube | spotify)
  import           Run an import (source = youtube, dest = spotify)
  status           Show which services are connected
  help             Show this message

Examples:
  musictui-import setup
  musictui-import auth youtube
  musictui-import auth spotify
  musictui-import import --include-liked
  musictui-import status

Config is stored under %s.
`, defaultConfigHint())
}

func defaultConfigHint() string {
	if dir, err := cliconfig.Dir(); err == nil {
		return dir
	}
	return filepath.Join("~", ".config", "musictui-import")
}

// flagSet builds a named FlagSet with a standard error-on-parse-
// failure behaviour.
func flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: musictui-import %s [flags]\n\n", name)
		fs.PrintDefaults()
	}
	return fs
}
