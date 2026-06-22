package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/model"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

type ModalKind int

const (
	ModalConfirm ModalKind = iota
	ModalInput
	ModalPicker
)

type ModalAction int

const (
	ActionCreatePlaylist ModalAction = iota
	ActionEditPlaylist
	ActionDeletePlaylist
	ActionAddToPlaylist
	ActionRemoveTrack
	ActionMoveTrack
	ActionConsolidateDuplicates
	ActionDeleteEmptyPlaylists
	ActionRestorePlaylists
)

type Modal struct {
	Active bool
	Kind   ModalKind
	Title  string
	// Confirm
	Message string
	// Input fields
	Input1      string
	Input1Label string
	Input2      string
	Input2Label string
	CursorPos1  int
	CursorPos2  int
	FocusField  int // 0 = field1, 1 = field2
	// Picker
	PickerItems    []model.Playlist
	PickerSelected int
	// Context
	Action   ModalAction
	TargetID string
	TrackURI string
}

func (m *Modal) ShowConfirm(title, message string, action ModalAction, targetID string) {
	m.Active = true
	m.Kind = ModalConfirm
	m.Title = title
	m.Message = message
	m.Action = action
	m.TargetID = targetID
}

func (m *Modal) ShowInput(title, label1, val1, label2, val2 string, action ModalAction, targetID string) {
	m.Active = true
	m.Kind = ModalInput
	m.Title = title
	m.Input1Label = label1
	m.Input1 = val1
	m.CursorPos1 = len(val1)
	m.Input2Label = label2
	m.Input2 = val2
	m.CursorPos2 = len(val2)
	m.FocusField = 0
	m.Action = action
	m.TargetID = targetID
}

func (m *Modal) ShowPicker(title string, playlists []model.Playlist, trackURI string) {
	m.Active = true
	m.Kind = ModalPicker
	m.Title = title
	m.PickerItems = playlists
	m.PickerSelected = 0
	m.TrackURI = trackURI
	m.Action = ActionAddToPlaylist
}

func (m *Modal) Close() {
	m.Active = false
	m.Message = ""
	m.Input1 = ""
	m.Input2 = ""
	m.PickerItems = nil
}

func (m *Modal) InputChar(r rune) {
	if m.FocusField == 0 {
		before := m.Input1[:m.CursorPos1]
		after := m.Input1[m.CursorPos1:]
		m.Input1 = before + string(r) + after
		m.CursorPos1 += len(string(r))
	} else {
		before := m.Input2[:m.CursorPos2]
		after := m.Input2[m.CursorPos2:]
		m.Input2 = before + string(r) + after
		m.CursorPos2 += len(string(r))
	}
}

func (m *Modal) Backspace() {
	if m.FocusField == 0 {
		if m.CursorPos1 > 0 {
			runes := []rune(m.Input1)
			runePos := len([]rune(m.Input1[:m.CursorPos1]))
			if runePos > 0 {
				newRunes := append(runes[:runePos-1], runes[runePos:]...)
				m.Input1 = string(newRunes)
				m.CursorPos1 = len(string(newRunes[:runePos-1]))
			}
		}
	} else {
		if m.CursorPos2 > 0 {
			runes := []rune(m.Input2)
			runePos := len([]rune(m.Input2[:m.CursorPos2]))
			if runePos > 0 {
				newRunes := append(runes[:runePos-1], runes[runePos:]...)
				m.Input2 = string(newRunes)
				m.CursorPos2 = len(string(newRunes[:runePos-1]))
			}
		}
	}
}

func (m *Modal) TabField() {
	if m.FocusField == 0 {
		m.FocusField = 1
	} else {
		m.FocusField = 0
	}
}

func (m *Modal) Up() {
	if m.PickerSelected > 0 {
		m.PickerSelected--
	}
}

func (m *Modal) Down() {
	if m.PickerSelected < len(m.PickerItems)-1 {
		m.PickerSelected++
	}
}

// View renders the modal content inside a bordered box.
func (m Modal) View(th theme.Theme, width, height int) string {
	boxW := 56
	if boxW > width-4 {
		boxW = width - 4
	}
	innerW := boxW - 4 // account for border + padding

	var content string
	switch m.Kind {
	case ModalConfirm:
		content = m.viewConfirm(th, innerW)
	case ModalInput:
		content = m.viewInput(th, innerW)
	case ModalPicker:
		content = m.viewPicker(th, innerW, height/2)
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Accent).
		Background(th.Surface).
		Foreground(th.Fg).
		Padding(0, 1).
		Width(boxW).
		Render(content)

	return box
}

// Overlay places the modal box centered on top of the base screen output.
func Overlay(base, modal string, screenW, screenH int) string {
	baseLines := strings.Split(base, "\n")
	modalLines := strings.Split(modal, "\n")

	modalH := len(modalLines)
	modalW := lipgloss.Width(modal)

	startY := (screenH - modalH) / 2
	startX := (screenW - modalW) / 2
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	// Pad base to fill screen height
	for len(baseLines) < screenH {
		baseLines = append(baseLines, "")
	}

	for i, mLine := range modalLines {
		y := startY + i
		if y >= len(baseLines) {
			break
		}
		baseRunes := []rune(stripAnsi(baseLines[y]))
		// Build: left padding from base + modal line + right padding from base
		left := ""
		if startX > 0 && startX <= len(baseRunes) {
			// Re-render the left portion from the original line
			left = padRight("", startX)
		} else {
			left = padRight("", startX)
		}
		baseLines[y] = left + mLine
	}
	return strings.Join(baseLines[:screenH], "\n")
}

// stripAnsi removes ANSI escape sequences for measuring rune length.
func stripAnsi(s string) string {
	var out []rune
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

func padRight(s string, w int) string {
	n := len([]rune(s))
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

func (m Modal) viewConfirm(th theme.Theme, width int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(m.Title)
	b.WriteString(title + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", width)) + "\n")
	b.WriteString("\n")

	// Wrap long messages
	for _, line := range strings.Split(m.Message, "\n") {
		b.WriteString(lipgloss.NewStyle().Foreground(th.Fg).Render(line) + "\n")
	}
	b.WriteString("\n")

	// Match the hint row format used everywhere else in the app.
	hint := lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true)
	keyY := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("y")
	keyN := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("n")
	keyEsc := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("Esc")
	b.WriteString(
		keyY + hint.Render(": yes  ·  ") +
			keyN + hint.Render(": no  ·  ") +
			keyEsc + hint.Render(": cancel"),
	)

	return b.String()
}

func (m Modal) viewInput(th theme.Theme, width int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(m.Title)
	b.WriteString(title + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", width)) + "\n")
	b.WriteString("\n")

	m.renderField(&b, th, m.Input1Label, m.Input1, m.CursorPos1, m.FocusField == 0, width)
	b.WriteString("\n")

	if m.Input2Label != "" {
		m.renderField(&b, th, m.Input2Label, m.Input2, m.CursorPos2, m.FocusField == 1, width)
		b.WriteString("\n")
	}

	hint := lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true)
	b.WriteString(hint.Render("Enter:submit  Tab:switch  Esc:cancel"))

	return b.String()
}

func (m Modal) renderField(b *strings.Builder, th theme.Theme, label, value string, cursorPos int, focused bool, width int) {
	labelStyle := lipgloss.NewStyle().Foreground(th.FgMuted)
	b.WriteString(labelStyle.Render(label) + "\n")

	fieldW := width - 2
	if fieldW < 10 {
		fieldW = 10
	}

	var display string
	if focused {
		before := value[:cursorPos]
		after := value[cursorPos:]
		cursor := lipgloss.NewStyle().Background(th.Accent).Render(" ")
		display = before + cursor + after
	} else {
		if value == "" {
			display = lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render("(empty)")
		} else {
			display = value
		}
	}

	borderColor := th.Border
	if focused {
		borderColor = th.Accent
	}
	fieldStyle := lipgloss.NewStyle().
		Foreground(th.Fg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(fieldW).
		Padding(0, 1)
	b.WriteString(fieldStyle.Render(display) + "\n")
}

func (m Modal) viewPicker(th theme.Theme, width, maxRows int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(m.Title)
	b.WriteString(title + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", width)) + "\n")

	if len(m.PickerItems) == 0 {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render("No playlists available") + "\n")
		return b.String()
	}

	visibleRows := maxRows
	if visibleRows < 3 {
		visibleRows = 3
	}
	if visibleRows > len(m.PickerItems) {
		visibleRows = len(m.PickerItems)
	}
	startIdx := 0
	if m.PickerSelected >= visibleRows {
		startIdx = m.PickerSelected - visibleRows + 1
	}
	endIdx := startIdx + visibleRows
	if endIdx > len(m.PickerItems) {
		endIdx = len(m.PickerItems)
	}

	for i := startIdx; i < endIdx; i++ {
		pl := m.PickerItems[i]
		name := truncate(pl.Name, width-4)
		if i == m.PickerSelected {
			indicator := lipgloss.NewStyle().Foreground(th.Accent).Render("▸ ")
			nameStr := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(name)
			b.WriteString(indicator + nameStr + "\n")
		} else {
			nameStr := lipgloss.NewStyle().Foreground(th.Fg).Render(name)
			b.WriteString("  " + nameStr + "\n")
		}
	}

	if len(m.PickerItems) > visibleRows {
		pos := fmt.Sprintf("%d / %d", m.PickerSelected+1, len(m.PickerItems))
		b.WriteString(lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render(pos) + "\n")
	}

	b.WriteString(lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render("Enter:select  Esc:cancel"))

	return b.String()
}
