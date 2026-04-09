package render

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"helmtrace/pkg/analyzer"
)

// styles
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")) // bright white

	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")) // bright blue

	missingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")) // dark grey

	redundantStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3")). // yellow
			Italic(true)

	effectiveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")). // bright green
			Bold(true)

	legendStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Italic(true)

	dividerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))
)

// TUITable renders a provenance table with lipgloss styling, fitting output
// to the terminal width. Long keys and values are truncated with an ellipsis.
// When nodes carry ResourceKey values (kustomize mode), output is split into
// one labelled section per resource.
func TUITable(nodes []analyzer.ValueNode, layers []analyzer.Layer) {
	groups := BuildGroups(nodes, layers)
	hasRedundant := false

	groupHeaderStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("14")) // bright cyan

	for i, g := range groups {
		if i > 0 {
			fmt.Println()
		}
		if g.ResourceKey != "" && g.ResourceKey != "_" {
			fmt.Println(groupHeaderStyle.Render(g.ResourceKey))
		}
		redundant := tuiSection(g.Nodes, g.Layers)
		if redundant {
			hasRedundant = true
		}
	}

	if hasRedundant {
		fmt.Println()
		fmt.Println(legendStyle.Render("✕ = redundant (identical to effective value from lower layers)"))
	}
}

// tuiSection renders one lipgloss-styled table section and returns true if
// any redundant values were found.
func tuiSection(nodes []analyzer.ValueNode, layers []analyzer.Layer) bool {
	termWidth := terminalWidth()

	layerNames := make([]string, len(layers))
	for i, l := range layers {
		layerNames[i] = l.Name
	}

	// Calculate natural column widths before truncation.
	keyWidth := len("KEY")
	for _, n := range nodes {
		if len(n.Key) > keyWidth {
			keyWidth = len(n.Key)
		}
	}
	colWidth := len("EFFECTIVE")
	for _, name := range layerNames {
		if len(name) > colWidth {
			colWidth = len(name)
		}
	}
	for _, n := range nodes {
		for _, s := range n.Sources {
			if w := len(fmt.Sprintf("%v", s.Value)); w > colWidth {
				colWidth = w
			}
		}
		if w := len(fmt.Sprintf("%v", n.EffectiveValue)); w > colWidth {
			colWidth = w
		}
	}

	// Fit to terminal width.
	numCols := 1 + len(layerNames) + 1
	colPad := 2
	available := termWidth - (numCols * colPad)
	if available < numCols*8 {
		available = numCols * 8
	}
	maxKeyWidth := available * 35 / 100
	if keyWidth > maxKeyWidth {
		keyWidth = maxKeyWidth
	}
	remaining := available - keyWidth
	if colWidth > remaining/(len(layerNames)+1) {
		colWidth = remaining / (len(layerNames) + 1)
	}
	if colWidth < 6 {
		colWidth = 6
	}

	colPadStr := strings.Repeat(" ", colPad)

	// Header row.
	row := headerStyle.Width(keyWidth + colPad).Render(truncate("KEY", keyWidth))
	for _, name := range layerNames {
		row += headerStyle.Width(colWidth + colPad).Render(truncate(name, colWidth))
	}
	row += headerStyle.Width(colWidth + colPad).Render(truncate("EFFECTIVE", colWidth))
	fmt.Println(row)

	// Divider.
	divLen := keyWidth + colPad + (colWidth+colPad)*(len(layerNames)+1)
	fmt.Println(dividerStyle.Render(strings.Repeat("─", divLen)))

	// Layer name → index for IsRedundant.
	layerIdx := map[string]int{}
	for i, l := range layers {
		layerIdx[l.Name] = i
	}

	hasRedundant := false
	for _, n := range nodes {
		sourceByLayer := map[string]analyzer.Source{}
		for _, s := range n.Sources {
			sourceByLayer[s.Layer] = s
		}

		row = keyStyle.Width(keyWidth + colPad).Render(truncate(n.Key, keyWidth)) + colPadStr

		for i, name := range layerNames {
			s, ok := sourceByLayer[name]
			if !ok {
				row += missingStyle.Width(colWidth + colPad).Render(truncate("—", colWidth))
				continue
			}
			cell := truncate(fmt.Sprintf("%v", s.Value), colWidth-2)
			if n.IsRedundant(i) {
				hasRedundant = true
				row += redundantStyle.Width(colWidth + colPad).Render(cell + " ✕")
			} else {
				row += lipgloss.NewStyle().Width(colWidth + colPad).Render(cell)
			}
		}

		eff := truncate(fmt.Sprintf("%v", n.EffectiveValue), colWidth)
		row += effectiveStyle.Width(colWidth + colPad).Render(eff)
		fmt.Println(row)
	}

	return hasRedundant
}

// truncate shortens s to maxLen runes, appending … if trimmed.
func truncate(s string, maxLen int) string {
	if maxLen < 1 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return string(runes[:maxLen-1]) + "…"
}

// terminalWidth returns the current terminal width, falling back to 120.
func terminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 120
	}
	return w
}
