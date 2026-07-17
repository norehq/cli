package output

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type Printer struct {
	JSON    bool
	NoColor bool
	Stderr  io.Writer
	Stdout  io.Writer
	Verbose bool
}

func (p Printer) Success(value any, human string) error {
	if p.JSON {
		encoder := json.NewEncoder(p.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	_, err := fmt.Fprintln(p.Stdout, human)
	return err
}

func (p Printer) Progress(value any, human string) {
	if p.JSON {
		_ = json.NewEncoder(p.Stderr).Encode(value)
		return
	}
	_, _ = fmt.Fprintln(p.Stdout, p.Info(human))
}

func (p Printer) Failure(value Error) {
	if p.JSON {
		_ = json.NewEncoder(p.Stderr).Encode(map[string]any{"error": value})
		return
	}
	_, _ = fmt.Fprintf(
		p.Stderr,
		"%s %s\n",
		p.style(
			p.Stderr,
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Red),
			"Error:",
		),
		value.Message,
	)
	if p.Verbose && value.Details != nil {
		encoded, err := json.MarshalIndent(value.Details, "", "  ")
		if err == nil {
			_, _ = fmt.Fprintf(p.Stderr, "\n%s\n", encoded)
		}
	}
}

func (p Printer) Info(value string) string {
	return p.status(p.Stdout, "→", lipgloss.Cyan, value)
}

func (p Printer) Done(value string) string {
	return p.status(p.Stdout, "✓", lipgloss.Green, value)
}

func (p Printer) Warning(value string) string {
	return p.status(p.Stdout, "⚠", lipgloss.Yellow, value)
}

func (p Printer) Heading(value string) string {
	return p.style(p.Stdout, lipgloss.NewStyle().Foreground(lipgloss.Cyan), value)
}

func (p Printer) Fields(entries [][2]string) string {
	width := 0
	for _, entry := range entries {
		if len(entry[0]) > width {
			width = len(entry[0])
		}
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		label := fmt.Sprintf("%-*s", width, entry[0])
		label = p.style(
			p.Stdout,
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Cyan),
			label,
		)
		value := entry[1]
		if strings.TrimSpace(value) == "" {
			value = "—"
		}
		lines = append(lines, label+"  "+value)
	}
	return strings.Join(lines, "\n")
}

func (p Printer) Table(columns []string, rows [][]string) string {
	if len(rows) == 0 {
		return "No results."
	}
	widths := make([]int, len(columns))
	for index, column := range columns {
		widths[index] = len(column)
	}
	for _, row := range rows {
		for index, value := range row {
			if index < len(widths) && len(value) > widths[index] {
				widths[index] = len(value)
			}
		}
	}
	line := func(values []string) string {
		cells := make([]string, len(columns))
		for index := range columns {
			value := "—"
			if index < len(values) && strings.TrimSpace(values[index]) != "" {
				value = values[index]
			}
			cells[index] = fmt.Sprintf("%-*s", widths[index], value)
		}
		return strings.Join(cells, "  ")
	}
	lines := []string{p.style(
		p.Stdout,
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Cyan),
		line(columns),
	)}
	for _, row := range rows {
		lines = append(lines, line(row))
	}
	return strings.Join(lines, "\n")
}

func (p Printer) status(writer io.Writer, symbol string, color color.Color, value string) string {
	return p.style(writer, lipgloss.NewStyle().Foreground(color), symbol) + " " + p.style(
		writer,
		lipgloss.NewStyle().Bold(true).Foreground(color),
		value,
	)
}

func (p Printer) style(writer io.Writer, style lipgloss.Style, value string) string {
	if p.NoColor || colorprofile.Detect(writer, os.Environ()) <= colorprofile.ASCII {
		return value
	}
	return style.Render(value)
}
