package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musictui-go/internal/theme"
)

// TitledPanelWithBg renders a titled panel with a custom background color.
func TitledPanelWithBg(title, content string, width, height int, borderColor lipgloss.Color, bgColor lipgloss.Color, th theme.Theme) string {
	focused := string(borderColor) == string(th.BorderFocused)
	return titledPanelImpl(title, content, width, height, borderColor, bgColor, th, focused)
}

func TitledPanel(title, content string, width, height int, borderColor lipgloss.Color, th theme.Theme) string {
	focused := string(borderColor) == string(th.BorderFocused)
	return titledPanelImpl(title, content, width, height, borderColor, th.Surface, th, focused)
}

// fillLine pads or truncates a line to exactly the given visual width.
// No background color — uses the terminal's own background.
func fillLine(line string, targetW int) string {
	w := lipgloss.Width(line)
	if w >= targetW {
		return lipgloss.NewStyle().MaxWidth(targetW).Render(line)
	}
	return line + strings.Repeat(" ", targetW-w)
}

func titledPanelImpl(title, content string, width, height int, borderColor lipgloss.Color, _ lipgloss.Color, th theme.Theme, focused bool) string {
	if width < 8 {
		width = 8
	}
	if height < 3 {
		height = 3
	}

	innerW := width - 2  // space between left/right border chars
	innerH := height - 2 // lines between top/bottom border

	bc := lipgloss.NewStyle().Foreground(borderColor)
	if focused {
		bc = bc.Bold(true)
	}
	titleStyle := lipgloss.NewStyle().Foreground(borderColor).Bold(true)

	titleText := titleStyle.Render(title)
	titleVisualW := lipgloss.Width(titleText)

	// ── Top border: ╭── TITLE ─────╮ ──
	dashesAfter := innerW - 4 - titleVisualW // "── " before title + " " after
	if dashesAfter < 1 {
		dashesAfter = 1
	}
	topLine := bc.Render("╭── ") + titleText + bc.Render(" "+strings.Repeat("─", dashesAfter)+"╮")

	// ── Bottom border ──
	bottomLine := bc.Render("╰" + strings.Repeat("─", innerW) + "╯")

	// ── Side borders + manually padded content lines ──
	side := bc.Render("│")

	contentLines := strings.Split(content, "\n")

	var bordered []string
	for i := 0; i < innerH; i++ {
		var line string
		if i < len(contentLines) {
			line = contentLines[i]
		}
		padded := fillLine(line, innerW)
		bordered = append(bordered, side+padded+side)
	}

	return topLine + "\n" + strings.Join(bordered, "\n") + "\n" + bottomLine
}

// OuterFrame wraps content in a full-terminal border with a centered title.
func OuterFrame(title, content string, width, height int, th theme.Theme) string {
	if width < 10 {
		width = 10
	}
	if height < 4 {
		height = 4
	}

	innerW := width - 2
	innerH := height - 2

	bc := lipgloss.NewStyle().Foreground(th.Border)
	titleStyle := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)

	titleText := titleStyle.Render(title)
	titleVisualW := lipgloss.Width(titleText)

	// Center the title in the top border
	leftDashes := (innerW - titleVisualW - 2) / 2
	if leftDashes < 2 {
		leftDashes = 2
	}
	rightDashes := innerW - leftDashes - titleVisualW - 2
	if rightDashes < 1 {
		rightDashes = 1
	}

	topLine := bc.Render("╭"+strings.Repeat("─", leftDashes)) + " " +
		titleText + " " +
		bc.Render(strings.Repeat("─", rightDashes)+"╮")

	bottomLine := bc.Render("╰" + strings.Repeat("─", innerW) + "╯")

	side := bc.Render("│")

	contentLines := strings.Split(content, "\n")

	var bordered []string
	for i := 0; i < innerH; i++ {
		var line string
		if i < len(contentLines) {
			line = contentLines[i]
		}
		padded := fillLine(line, innerW)
		bordered = append(bordered, side+padded+side)
	}

	return topLine + "\n" + strings.Join(bordered, "\n") + "\n" + bottomLine
}

// PanelSection defines a section within a multi-section column panel.
type PanelSection struct {
	Title   string
	Content string
	Lines   int // number of INNER content lines (excluding divider)
}

// MultiSectionColumn renders a single bordered column with multiple titled sections
// separated by internal dividers (├── TITLE ──┤). This avoids gaps between stacked panels.
//
//	╭── SECTION1 ──╮
//	│ content       │
//	├── SECTION2 ──┤
//	│ content       │
//	├── SECTION3 ──┤
//	│ content       │
//	╰──────────────╯
func MultiSectionColumn(sections []PanelSection, width, height int, borderColor lipgloss.Color, bgColor lipgloss.Color, th theme.Theme) string {
	if width < 8 {
		width = 8
	}
	if height < 3 {
		height = 3
	}

	innerW := width - 2

	focused := string(borderColor) == string(th.BorderFocused)
	bc := lipgloss.NewStyle().Foreground(borderColor)
	if focused {
		bc = bc.Bold(true)
	}
	titleStyle := lipgloss.NewStyle().Foreground(borderColor).Bold(true)
	side := bc.Render("│")

	var allLines []string

	for i, sec := range sections {
		titleText := titleStyle.Render(sec.Title)
		titleVisualW := lipgloss.Width(titleText)

		dashesAfter := innerW - 4 - titleVisualW
		if dashesAfter < 1 {
			dashesAfter = 1
		}

		if i == 0 {
			// First section: top border ╭── TITLE ──╮
			allLines = append(allLines,
				bc.Render("╭── ")+titleText+bc.Render(" "+strings.Repeat("─", dashesAfter)+"╮"))
		} else {
			// Subsequent sections: divider ├── TITLE ──┤
			allLines = append(allLines,
				bc.Render("├── ")+titleText+bc.Render(" "+strings.Repeat("─", dashesAfter)+"┤"))
		}

		// Content lines for this section
		contentLines := strings.Split(sec.Content, "\n")
		for j := 0; j < sec.Lines; j++ {
			var line string
			if j < len(contentLines) {
				line = contentLines[j]
			}
			allLines = append(allLines, side+fillLine(line, innerW)+side)
		}
	}

	// Bottom border
	allLines = append(allLines, bc.Render("╰"+strings.Repeat("─", innerW)+"╯"))

	// Ensure exact height: trim or pad
	totalLines := len(allLines)
	if totalLines > height {
		allLines = allLines[:height-1]
		allLines = append(allLines, bc.Render("╰"+strings.Repeat("─", innerW)+"╯"))
	}
	for len(allLines) < height {
		// Insert padding before bottom border
		padLine := side + fillLine("", innerW) + side
		allLines = append(allLines[:len(allLines)-1], padLine, allLines[len(allLines)-1])
	}

	return strings.Join(allLines, "\n")
}

// StatusBar renders a single-line status bar.
func StatusBar(left, right string, width int, th theme.Theme) string {
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := width - leftW - rightW
	if gap < 1 {
		gap = 1
	}

	return left + strings.Repeat(" ", gap) + right
}
