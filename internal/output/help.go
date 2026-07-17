package output

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type helpItem struct {
	description string
	name        string
}

func (p Printer) Help(command *cobra.Command, version string) error {
	description := command.Long
	if strings.TrimSpace(description) == "" {
		description = command.Short
	}
	commands := helpCommands(command)
	sections := []string{
		p.helpHeader(command, version, description),
		p.helpSection(
			"Usage",
			"  "+p.style(
				p.Stdout,
				lipgloss.NewStyle().Foreground(lipgloss.Cyan),
				"$",
			)+" "+strings.ReplaceAll(command.UseLine(), "[flags]", "[options]"),
		),
	}
	if len(commands) > 0 {
		sections = append(sections, p.helpSection("Commands", p.helpRows(commands)))
	}
	if options := helpFlags(command.NonInheritedFlags()); len(options) > 0 {
		sections = append(sections, p.helpSection("Options", p.helpRows(options)))
	}
	if options := helpFlags(command.InheritedFlags()); len(options) > 0 {
		sections = append(sections, p.helpSection("Global options", p.helpRows(options)))
	}
	if examples := helpExamples(command.Example); len(examples) > 0 {
		sections = append(sections, p.helpSection("Examples", "  "+strings.Join(examples, "\n  ")))
	}
	if len(commands) > 0 {
		sections = append(
			sections,
			p.style(
				p.Stdout,
				lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
				fmt.Sprintf(
					"Run `%s <command> --help` for more information.",
					command.CommandPath(),
				),
			),
		)
	}
	_, err := fmt.Fprintln(p.Stdout, strings.Join(sections, "\n\n"))
	return err
}

func (p Printer) helpHeader(command *cobra.Command, version string, description string) string {
	title := p.style(
		p.Stdout,
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Magenta),
		command.CommandPath(),
	)
	versionText := p.style(
		p.Stdout,
		lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
		helpVersion(version),
	)
	return title + " " + versionText + "\n" + description
}

func (p Printer) helpSection(title string, content string) string {
	return p.style(
		p.Stdout,
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Cyan),
		title,
	) + "\n" + content
}

func (p Printer) helpRows(items []helpItem) string {
	width := 0
	for _, item := range items {
		if len(item.name) > width {
			width = len(item.name)
		}
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		name := item.name + strings.Repeat(" ", width-len(item.name))
		lines = append(
			lines,
			p.style(
				p.Stdout,
				lipgloss.NewStyle().Foreground(lipgloss.Green),
				name,
			)+"  "+item.description,
		)
	}
	return strings.Join(lines, "\n")
}

func helpCommands(command *cobra.Command) []helpItem {
	items := make([]helpItem, 0, len(command.Commands()))
	for _, child := range command.Commands() {
		if !child.IsAvailableCommand() || child.Hidden || child.Name() == "help" {
			continue
		}
		items = append(items, helpItem{
			description: child.Short,
			name:        strings.ReplaceAll(child.Use, "[flags]", "[options]"),
		})
	}
	return items
}

func helpFlags(flags *pflag.FlagSet) []helpItem {
	items := make([]helpItem, 0)
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		items = append(items, helpItem{
			description: flag.Usage + helpFlagDefault(flag),
			name:        helpFlagLabel(flag),
		})
	})
	return items
}

func helpFlagLabel(flag *pflag.Flag) string {
	name := "    --" + flag.Name
	if flag.Shorthand != "" {
		name = "  -" + flag.Shorthand + ", --" + flag.Name
	}
	if flag.NoOptDefVal == "" {
		name += " <" + helpFlagType(flag) + ">"
	}
	return name
}

func helpFlagType(flag *pflag.Flag) string {
	switch flag.Value.Type() {
	case "int", "int32", "int64", "uint", "uint32", "uint64":
		return "number"
	default:
		return flag.Value.Type()
	}
}

func helpFlagDefault(flag *pflag.Flag) string {
	if flag.Value.Type() == "bool" {
		return ""
	}
	switch flag.DefValue {
	case "", "0", "0s":
		return ""
	}
	if flag.Value.Type() == "string" {
		return " (default: " + strconv.Quote(flag.DefValue) + ")"
	}
	return " (default: " + flag.DefValue + ")"
}

func helpExamples(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(value), "\n")
	for index, line := range lines {
		lines[index] = strings.TrimSpace(line)
	}
	return lines
}

func helpVersion(version string) string {
	if version == "" || version == "dev" {
		return "dev"
	}
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}
