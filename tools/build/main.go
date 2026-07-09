// Command build compiles musicTUI the same way on every platform: it builds the
// Rust player-bridge, drops it into bridge-bin/ so //go:embed picks it up, and
// links the Go binary with the version stamped in.
//
// Windows has no make, and the Makefile's mkdir -p / cp / cd && … are POSIX
// shell, so `make build` was Unix-only. That left Windows with no supported way
// to build from a clone: `go build` alone silently produces a binary with no
// audio engine. This runs anywhere Go runs, and the Makefile delegates to it so
// the two can't drift.
//
//	go run ./tools/build          # -> dist/musicTUI[.exe] with the bridge embedded
//	go run ./tools/build test     # go test ./...  +  cargo test
//	go run ./tools/build clean
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const appName = "musicTUI"

func main() {
	task := "build"
	if len(os.Args) > 1 {
		task = os.Args[1]
	}

	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		fatal(err)
	}

	switch task {
	case "build":
		err = build()
	case "test":
		err = test()
	case "clean":
		err = clean()
	default:
		err = fmt.Errorf("unknown task %q (want: build, test, clean)", task)
	}
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "\nbuild: %v\n", err)
	os.Exit(1)
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// repoRoot walks up from the working directory looking for go.mod, so the
// script works from any subdirectory.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod found — run this from inside the repository")
		}
		dir = parent
	}
}

// requireTool turns a missing toolchain into an actionable message rather than
// an "executable file not found in $PATH" a few frames down.
func requireTool(name, why, install string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s not found on PATH — needed to %s\n  install: %s", name, why, install)
	}
	return nil
}

func run(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

// version mirrors the Makefile: v0.3.0-6-gabc1234, or -dirty when the tree has
// uncommitted changes, so `musicTUI --version` names the exact build.
func version() string {
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return "dev"
	}
	if v := strings.TrimSpace(string(out)); v != "" {
		return v
	}
	return "dev"
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func buildBridge() error {
	if err := requireTool("cargo", "build the Rust audio engine",
		"https://rustup.rs  (Windows also needs the Visual Studio C++ build tools)"); err != nil {
		return err
	}

	fmt.Println("==> building player-bridge (Rust, release)")
	if err := run("bridge", "cargo", "build", "--bin", "player-bridge", "--release"); err != nil {
		if runtime.GOOS == "linux" {
			return fmt.Errorf("%w\n  on Linux this usually means the ALSA headers are missing: sudo apt-get install libasound2-dev", err)
		}
		return err
	}

	bridge := "player-bridge" + exeSuffix()
	if err := os.MkdirAll("bridge-bin", 0o755); err != nil {
		return err
	}
	src := filepath.Join("bridge", "target", "release", bridge)
	dst := filepath.Join("bridge-bin", bridge)
	if err := copyFile(src, dst); err != nil {
		return fmt.Errorf("embed bridge: %w", err)
	}
	fmt.Printf("    embedded %s\n", dst)
	return nil
}

func build() error {
	if err := requireTool("go", "compile musicTUI", "https://go.dev/dl/"); err != nil {
		return err
	}
	if err := buildBridge(); err != nil {
		return err
	}

	if err := os.MkdirAll("dist", 0o755); err != nil {
		return err
	}
	out := filepath.Join("dist", appName+exeSuffix())
	v := version()

	fmt.Println("==> building musicTUI (Go, bridge embedded)")
	if err := run(".", "go", "build",
		"-ldflags", "-s -w -X main.Version="+v,
		"-o", out, "."); err != nil {
		return err
	}

	fmt.Printf("\nBuilt: %s %s (audio engine embedded)\n", out, v)
	return nil
}

func test() error {
	fmt.Println("==> go test ./...")
	if err := run(".", "go", "test", "./..."); err != nil {
		return err
	}
	if _, err := exec.LookPath("cargo"); err != nil {
		fmt.Println("\ncargo not found — skipping the Rust bridge tests")
		return nil
	}
	fmt.Println("\n==> cargo test (bridge)")
	return run("bridge", "cargo", "test")
}

// clean removes build output but keeps bridge-bin/.gitkeep, which //go:embed
// needs in order to match at all on a fresh clone.
func clean() error {
	if err := os.RemoveAll("dist"); err != nil {
		return err
	}
	for _, name := range []string{"player-bridge", "player-bridge.exe"} {
		if err := os.Remove(filepath.Join("bridge-bin", name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	fmt.Println("Cleaned dist/ and bridge-bin/")
	return nil
}
