package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

// Help is the full-screen keyboard reference, reachable with `?` from
// anywhere in the app. Scrollable with j/k when the content exceeds
// the panel height; closed with Esc.
type Help struct {
	ScrollPos int
}

func NewHelp() Help {
	return Help{}
}

func (h *Help) ScrollUp() {
	if h.ScrollPos > 0 {
		h.ScrollPos--
	}
}

func (h *Help) ScrollDown() {
	h.ScrollPos++
}

func (h *Help) Reset() {
	h.ScrollPos = 0
}

type helpGroup struct {
	title string
	hints []Hint
}

// helpGroups is the single source of truth for user-facing keybindings.
// Edit here to update both the ? screen and (eventually) any other
// rendered keybinding reference.
var helpGroups = []helpGroup{
	{
		"Global",
		[]Hint{
			{"Ctrl+C", "quit immediately"},
			{"q", "quit (not while typing)"},
			{"Ctrl+L", "log in / open setup wizard"},
			{"Ctrl+U", "install available update"},
			{"?", "open this help"},
			{"Esc", "close help / go back"},
		},
	},
	{
		"Navigation",
		[]Hint{
			{"j / Down", "move down"},
			{"k / Up", "move up"},
			{"Enter", "select / play"},
			{"Esc / h", "go back"},
			{"Tab", "next panel"},
			{"Shift+Tab", "previous panel"},
			{"Left / Right", "switch panels"},
			{"/", "jump to search"},
		},
	},
	{
		"Playback",
		[]Hint{
			{"Space", "play / pause"},
			{"n", "next track"},
			{"p", "previous track"},
			{"+ / =", "volume up"},
			{"-", "volume down"},
			{"s", "toggle shuffle"},
			{"r", "cycle repeat mode"},
			{"l", "toggle inline lyrics"},
		},
	},
	{
		"Library / Search / Playlists",
		[]Hint{
			{"Enter", "play selected track"},
			{"a", "add track to a playlist"},
			{"d", "remove track from playlist"},
			{"m", "move track to another playlist"},
		},
	},
	{
		"Playlist management (in sidebar)",
		[]Hint{
			{"c", "create new playlist"},
			{"e", "edit playlist name / description"},
			{"d", "remove from library (unfollow)"},
			{"Shift+R", "restore unfollowed playlists from last backup"},
		},
	},
	{
		"Popups / modals",
		[]Hint{
			{"Enter", "confirm / submit"},
			{"Esc", "cancel"},
			{"Tab", "switch input field"},
			{"y", "yes (confirm modals)"},
			{"n", "no (confirm modals)"},
		},
	},
}

// View renders the help screen. width is the content area width; height
// is the usable rows before the status bar.
func (h Help) View(th theme.Theme, width, height int) string {
	title := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).
		Render("Keyboard Reference")
	hint := lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).
		Render("j/k: scroll · Esc: close")

	groupTitle := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Underline(true)
	keyStyle := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(th.Fg)

	// Render every group into a list of lines first so we can scroll.
	var lines []string
	for gi, g := range helpGroups {
		if gi > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, " "+groupTitle.Render(g.title))
		for _, kv := range g.hints {
			key := keyStyle.Width(14).Render(kv.Key)
			desc := descStyle.Render(kv.Desc)
			lines = append(lines, "   "+key+"  "+desc)
		}
	}

	// Reserve 3 rows at the top for title + blank + scroll hint line
	reserved := 3
	viewH := height - reserved
	if viewH < 3 {
		viewH = 3
	}

	start := h.ScrollPos
	if start > len(lines)-viewH {
		start = len(lines) - viewH
	}
	if start < 0 {
		start = 0
	}
	end := start + viewH
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	b.WriteString(" " + title + "   " + hint + "\n")
	b.WriteString(" " + lipgloss.NewStyle().Foreground(th.Border).
		Render(strings.Repeat("─", width-2)) + "\n")
	b.WriteString(strings.Join(lines[start:end], "\n"))

	return b.String()
}
