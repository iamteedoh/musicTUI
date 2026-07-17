package components

import (
	"fmt"
	"hash/crc32"
	"image"
	"strings"
	"testing"
	"time"
)

func TestKittyGraphicsReplies(t *testing.T) {
	// Two complete replies with unrelated terminal chatter between them.
	buf := []byte("\x1b_Gi=42;OK\x1b\\\x1b[?62;4c\x1b_Gi=42;EFBIG:too large\x1b\\")
	got := kittyGraphicsReplies(buf)
	want := []string{"i=42;OK", "i=42;EFBIG:too large"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("kittyGraphicsReplies = %q, want %q", got, want)
	}

	// An incomplete trailing reply must be held back, not half-returned.
	partial := kittyGraphicsReplies([]byte("\x1b_Gi=42;OK\x1b\\\x1b_Gi=42;EIN"))
	if len(partial) != 1 || partial[0] != "i=42;OK" {
		t.Fatalf("partial tail: got %q, want just the complete reply", partial)
	}

	if got := kittyGraphicsReplies([]byte("no escapes here")); len(got) != 0 {
		t.Fatalf("plain text produced replies: %q", got)
	}
}

func TestKittyImageID(t *testing.T) {
	// crc32("") is 0 — the protocol's "no id" — so it must map to 1.
	if got := kittyImageID(""); got != 1 {
		t.Fatalf("kittyImageID(\"\") = %d, want 1", got)
	}
	url := "https://i.scdn.co/image/ab67616d0000b273deadbeef"
	want := crc32.ChecksumIEEE([]byte(url)) & 0xFFFFFF
	if got := kittyImageID(url); got != want {
		t.Fatalf("kittyImageID = %d, want %d", got, want)
	}
}

// The app transmits with q=2; the probe with quiet=0, where the q key is
// omitted entirely (verbose is the protocol default). The q=2 wire format is
// load-bearing — it is what every kitty/Ghostty user's session has always
// sent — so it must stay byte-identical.
func TestKittyTransmitQuiet(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))

	seq2, stats2, err := kittyTransmit(7, img, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(seq2, "\x1b_Ga=t,q=2,f=100,i=7,m=") {
		t.Fatalf("q=2 first chunk changed shape: %q", seq2[:30])
	}
	if stats2.Chunks != 1 || stats2.PNGBytes == 0 || stats2.B64Chars == 0 {
		t.Fatalf("tiny image stats: %+v", stats2)
	}

	seq0, _, err := kittyTransmit(7, img, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(seq0, "\x1b_Ga=t,f=100,i=7,m=") {
		t.Fatalf("verbose first chunk shape: %q", seq0[:30])
	}
	if strings.Contains(seq0, "q=") {
		t.Fatalf("verbose transmit must omit the q key entirely: %q", seq0[:40])
	}
}

func TestKittyTransmitChunking(t *testing.T) {
	img := ProbeTestImage() // large enough for a multi-chunk payload
	seq, stats, err := kittyTransmit(9, img, 2)
	if err != nil {
		t.Fatal(err)
	}
	wantChunks := (stats.B64Chars + 4095) / 4096
	if stats.Chunks != wantChunks || stats.Chunks < 2 {
		t.Fatalf("chunks = %d, want %d (>1)", stats.Chunks, wantChunks)
	}
	// Continuation chunks carry the quiet key in q=2 mode (the historical
	// wire format) …
	if !strings.Contains(seq, "\x1b\\\x1b_Gq=2,m=") {
		t.Fatal("q=2 continuation chunk changed shape")
	}
	// … and reassembling every chunk payload must yield the full base64.
	total := 0
	for _, part := range strings.Split(seq, "\x1b\\") {
		if i := strings.IndexByte(part, ';'); i >= 0 {
			total += len(part) - i - 1
		}
	}
	if total != stats.B64Chars {
		t.Fatalf("reassembled %d base64 chars, want %d", total, stats.B64Chars)
	}

	seq0, _, err := kittyTransmit(9, img, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seq0, "\x1b\\\x1b_Gm=") {
		t.Fatal("verbose continuation chunk should carry only the m key")
	}
}

func TestKittyGridSize(t *testing.T) {
	// A square cover fills the probe's nominal 24×12 panel exactly.
	if c, r := kittyGridSize(640, 640, 24, 12); c != 24 || r != 12 {
		t.Fatalf("square: got %dx%d, want 24x12", c, r)
	}
	// A panel larger than the diacritic table caps at 40 per axis.
	if c, r := kittyGridSize(1000, 1000, 100, 50); c != 40 || r != 40 {
		t.Fatalf("cap: got %dx%d, want 40x40", c, r)
	}
	// Degenerate sources still address at least one cell.
	if c, _ := kittyGridSize(1, 1000, 20, 10); c != 1 {
		t.Fatalf("min width: got cols=%d, want 1", c)
	}
}

// fakeProbeTTY scripts the terminal side of a probe run: it records what was
// written and answers each RoundTrip from a canned queue.
type fakeProbeTTY struct {
	replies [][]byte
	writes  []string
}

func (f *fakeProbeTTY) RoundTrip(payload string, _ func([]byte) bool, _ time.Duration) ([]byte, error) {
	f.writes = append(f.writes, payload)
	var r []byte
	if len(f.replies) > 0 {
		r = f.replies[0]
		f.replies = f.replies[1:]
	}
	return r, nil
}

func TestProbeKittyArtworkAccepted(t *testing.T) {
	src := "https://example.test/cover.jpg"
	id := kittyImageID(src)
	tty := &fakeProbeTTY{replies: [][]byte{
		[]byte(fmt.Sprintf("\x1b_Gi=%d;OK\x1b\\\x1b_Gi=%d;OK\x1b\\", id, id)),
	}}

	r := ProbeKittyArtwork(src, image.NewRGBA(image.Rect(0, 0, 8, 8)), tty)

	if r.ID != id {
		t.Fatalf("ID = %d, want %d", r.ID, id)
	}
	if !r.TransmitOK() || !r.PlacementOK() {
		t.Fatalf("verdicts: transmit=%q placement=%q", r.TransmitReply, r.PlacementReply)
	}
	if len(tty.writes) != 1 {
		t.Fatalf("wrote %d rounds, want exactly one (transmit+placement)", len(tty.writes))
	}
	// A verbose transmit and a verbose virtual placement, in that order.
	if !strings.Contains(tty.writes[0], fmt.Sprintf("\x1b_Ga=t,f=100,i=%d,", id)) {
		t.Fatalf("missing verbose transmit: %q", tty.writes[0][:40])
	}
	if !strings.Contains(tty.writes[0], fmt.Sprintf("\x1b_Ga=p,U=1,i=%d,", id)) {
		t.Fatal("missing verbose placement")
	}
	if grid := r.PlaceholderGrid(); strings.Count(grid, "\n") != r.Rows-1 {
		t.Fatalf("grid rows = %d, want %d", strings.Count(grid, "\n")+1, r.Rows)
	}
}

// The probe must never delete the image it just transmitted: a=d,d=I frees the
// image AND its virtual placement, so the placeholder grid the caller prints
// next would reference nothing and render as literal glyphs — making a healthy
// terminal look broken. This shipped once; the grid was blank in Ghostty for
// exactly this reason.
func TestProbeKittyArtworkNeverDeletesTheImageItRendered(t *testing.T) {
	src := "https://example.test/cover.jpg"
	id := kittyImageID(src)
	tty := &fakeProbeTTY{replies: [][]byte{
		[]byte(fmt.Sprintf("\x1b_Gi=%d;OK\x1b\\\x1b_Gi=%d;OK\x1b\\", id, id)),
	}}

	ProbeKittyArtwork(src, image.NewRGBA(image.Rect(0, 0, 8, 8)), tty)

	for _, w := range tty.writes {
		if strings.Contains(w, "\x1b_Ga=d,") {
			t.Fatalf("probe deleted the image it asked the user to look at: %q", w)
		}
	}
}

func TestProbeKittyArtworkRejected(t *testing.T) {
	src := "cover"
	id := kittyImageID(src)
	tty := &fakeProbeTTY{replies: [][]byte{
		[]byte(fmt.Sprintf("\x1b_Gi=%d;EFBIG:image too large\x1b\\", id)),
	}}

	r := ProbeKittyArtwork(src, image.NewRGBA(image.Rect(0, 0, 8, 8)), tty)

	if r.TransmitOK() {
		t.Fatal("a rejection must not read as OK")
	}
	if !strings.Contains(r.TransmitReply, "EFBIG") {
		t.Fatalf("TransmitReply = %q, want the terminal's error", r.TransmitReply)
	}
	if r.PlacementReply != "" {
		t.Fatalf("PlacementReply = %q, want empty (no second reply)", r.PlacementReply)
	}
}

func TestProbeKittyArtworkSilentTerminal(t *testing.T) {
	tty := &fakeProbeTTY{} // never answers anything
	r := ProbeKittyArtwork("cover", image.NewRGBA(image.Rect(0, 0, 8, 8)), tty)
	if r.TransmitReply != "" || r.PlacementReply != "" || r.TransmitOK() {
		t.Fatalf("silent terminal produced verdicts: %+v", r)
	}
}

func TestProbeTestImageDeterministic(t *testing.T) {
	a, b := ProbeTestImage(), ProbeTestImage()
	if a.Bounds().Dx() != 640 || a.Bounds().Dy() != 640 {
		t.Fatalf("bounds = %v, want 640x640", a.Bounds())
	}
	for _, p := range []image.Point{{0, 0}, {639, 0}, {320, 320}, {639, 639}} {
		if a.At(p.X, p.Y) != b.At(p.X, p.Y) {
			t.Fatalf("not deterministic at %v", p)
		}
	}
}

// TestKittyPlaceholderRowColorFidelity is the MUS-34 regression guard.
//
// The placeholder row encodes the 24-bit image id as a foreground color. Before
// the fix this went through lipgloss → termenv → go-colorful which converts
// hex → float64 via (1.0/255.0) → uint8 via (*255). Because 1/255 is not
// exactly representable in binary64, 24 of 256 byte values round-trip to b−1,
// corrupting the id. The terminal then looks up an image it never received and
// renders nothing. This test exhausts all 256 values in all three byte positions
// and asserts each is emitted and reconstructed exactly.
func TestKittyPlaceholderRowColorFidelity(t *testing.T) {
	for v := 0; v < 256; v++ {
		// Keep the two non-tested channels at fixed non-zero values (0x55, 0xAA)
		// so the composite id is never zero (the protocol's "no id").
		cases := []struct {
			ch string
			id uint32
		}{
			{"R", uint32(v)<<16 | 0x5500AA},
			{"G", 0x550000 | uint32(v)<<8 | 0xAA},
			{"B", 0x55AA00 | uint32(v)},
		}
		for _, tc := range cases {
			row := kittyPlaceholderRow(tc.id, 0, 1)
			wantR := (tc.id >> 16) & 0xff
			wantG := (tc.id >> 8) & 0xff
			wantB := tc.id & 0xff
			params := extractSGRParams(row)
			if params == "" {
				t.Fatalf("ch=%s v=%d id=%d: no SGR escape found in %q", tc.ch, v, tc.id, row)
			}
			var gotR, gotG, gotB uint32
			if _, err := fmt.Sscanf(params, "38;2;%d;%d;%d", &gotR, &gotG, &gotB); err != nil {
				t.Fatalf("ch=%s v=%d id=%d: cannot parse SGR %q: %v", tc.ch, v, tc.id, params, err)
			}
			if gotR != wantR || gotG != wantG || gotB != wantB {
				t.Errorf("ch=%s v=%d id=%d: emitted %d;%d;%d, want %d;%d;%d — id corrupted (MUS-34)",
					tc.ch, v, tc.id, gotR, gotG, gotB, wantR, wantG, wantB)
			}
		}
	}
}

// extractSGRParams returns the parameter string between the first ESC[ and its
// closing m — e.g. "38;2;22;163;110" from "\x1b[38;2;22;163;110m...".
func extractSGRParams(s string) string {
	i := strings.Index(s, "\x1b[")
	if i < 0 {
		return ""
	}
	s = s[i+2:]
	j := strings.IndexByte(s, 'm')
	if j < 0 {
		return ""
	}
	return s[:j]
}
