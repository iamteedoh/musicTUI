package components

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtworkDebugLogWritesToPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artwork.log")
	t.Setenv("MUSICTUI_ARTWORK_DEBUG", path)
	artworkDebugReset()
	t.Cleanup(artworkDebugReset)

	artworkDebugf("hello %d", 42)

	if got := ArtworkDebugPath(); got != path {
		t.Fatalf("ArtworkDebugPath = %q, want %q", got, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello 42") {
		t.Fatalf("log content = %q, want it to contain the formatted line", data)
	}
}

func TestArtworkDebugDisabledByDefault(t *testing.T) {
	for _, v := range []string{"", "0", "false", "off"} {
		t.Setenv("MUSICTUI_ARTWORK_DEBUG", v)
		artworkDebugReset()
		t.Cleanup(artworkDebugReset)

		artworkDebugf("must go nowhere")
		if got := ArtworkDebugPath(); got != "" {
			t.Fatalf("MUSICTUI_ARTWORK_DEBUG=%q: path = %q, want disabled", v, got)
		}
	}
}
