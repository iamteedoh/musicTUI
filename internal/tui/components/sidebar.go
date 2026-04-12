package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/model"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

type Sidebar struct {
	Items    []model.SidebarItem
	Selected int
}

func NewSidebar() Sidebar {
	return Sidebar{
		Items:    model.SidebarItems,
		Selected: 0,
	}
}

func (s *Sidebar) Up() {
	if s.Selected > 0 {
		s.Selected--
	} else {
		s.Selected = len(s.Items) - 1
	}
}

func (s *Sidebar) Down() {
	if s.Selected < len(s.Items)-1 {
		s.Selected++
	} else {
		s.Selected = 0
	}
}

func (s *Sidebar) CurrentView() model.View {
	return s.Items[s.Selected].View
}

// ViewContent renders the sidebar content (nav items) without any border.
// The border is handled by TitledPanel in the app layout.
func (s Sidebar) ViewContent(th theme.Theme, width, height int) string {
	var items []string
	for i, item := range s.Items {
		if i == s.Selected {
			indicator := lipgloss.NewStyle().
				Foreground(th.Accent).
				Render("▐")

			label := lipgloss.NewStyle().
				Foreground(th.Accent).
				Bold(true).
				Background(th.SidebarSel).
				Width(width - 1).
				Render(" " + item.Icon + item.Name)

			items = append(items, indicator+label)
		} else {
			label := lipgloss.NewStyle().
				Foreground(th.FgDim).
				Width(width).
				Render("  " + item.Icon + item.Name)

			items = append(items, label)
		}
	}

	return strings.Join(items, "\n")
}
