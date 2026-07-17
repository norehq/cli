package command

import (
	"errors"
	"os"
	"strings"

	"github.com/norehq/cli/internal/home"
	"github.com/norehq/cli/internal/skillinstall"
	bundledskill "github.com/norehq/cli/skill"
	"github.com/spf13/cobra"
)

type updateSkillData struct {
	CLIVersion   string                `json:"cliVersion"`
	SkillVersion string                `json:"skillVersion"`
	Targets      []skillinstall.Target `json:"targets"`
}

type showSkillData struct {
	CLIVersion   string `json:"cliVersion"`
	SkillVersion string `json:"skillVersion"`
	Text         string `json:"text"`
}

func (a *app) skillCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "skill [command]",
		Short: "Manage the bundled Nore skill",
		Long:  "Create or replace the bundled Nore skill for supported coding agents, or print its text.",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(a.skillUpdateCommand(), a.skillShowCommand())
	return command
}

func (a *app) skillUpdateCommand() *cobra.Command {
	var client string
	command := &cobra.Command{
		Use:   "update",
		Short: "Create or replace the bundled skill for coding agents",
		Long: "Create or replace the user-level Nore skill for Codex, Claude Code, Cursor, " +
			"or every detected supported client with the version bundled in this CLI.",
		Args: cobra.NoArgs,
		Example: `  nore skill update
  nore skill update --client codex
  nore skill update --client all --json`,
		RunE: func(_ *cobra.Command, _ []string) error {
			userHome, err := os.UserHomeDir()
			if err != nil {
				return newCommandError("SKILL_HOME_UNAVAILABLE", "The user home directory could not be resolved.", err.Error())
			}
			targets, err := skillinstall.Targets(userHome, client)
			switch {
			case errors.Is(err, skillinstall.ErrUnsupportedClient):
				return newCommandError(
					"UNSUPPORTED_CLIENT",
					"Unsupported skill client: "+client+".",
					map[string]any{
						"supported": skillinstall.SupportedClients(),
						"value":     client,
					},
				)
			case errors.Is(err, skillinstall.ErrNoClientDetected):
				return newCommandError(
					"SKILL_CLIENT_NOT_DETECTED",
					"No supported coding agent was detected. Pass --client with codex, claude, cursor, or all.",
					map[string]any{"supported": skillinstall.SupportedClients()},
				)
			case err != nil:
				return newCommandError("SKILL_CLIENT_DETECTION_FAILED", "Coding agent detection failed.", err.Error())
			}
			bundle, err := bundledskill.Bundle()
			if err != nil {
				return newCommandError("SKILL_BUNDLE_UNAVAILABLE", "The bundled Nore skill could not be read.", err.Error())
			}
			lines := make([]string, 0, len(targets))
			for _, target := range targets {
				if err := skillinstall.Install(bundle, target); err != nil {
					return newCommandError(
						"SKILL_INSTALL_FAILED",
						"The bundled Nore skill could not be installed.",
						map[string]any{
							"client": target.Client,
							"error":  err.Error(),
							"path":   target.Path,
						},
					)
				}
				lines = append(lines, a.printer().Done(
					"Skill updated: "+target.DisplayName+" · "+home.DisplayPath(target.Path),
				))
			}
			return a.printer().Success(updateSkillData{
				CLIVersion:   a.buildInfo.Version,
				SkillVersion: bundledskill.Version,
				Targets:      targets,
			}, strings.Join(lines, "\n"))
		},
	}
	command.Flags().StringVar(
		&client,
		"client",
		skillinstall.ClientAuto,
		"target client: auto, codex, claude, cursor, or all",
	)
	command.Flags().SortFlags = false
	return command
}

func (a *app) skillShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the bundled skill text",
		Args:  cobra.NoArgs,
		Example: `  nore skill show
  nore skill show --json`,
		RunE: func(_ *cobra.Command, _ []string) error {
			text, err := bundledskill.Text()
			if err != nil {
				return newCommandError("SKILL_BUNDLE_UNAVAILABLE", "The bundled Nore skill could not be read.", err.Error())
			}
			return a.printer().Success(showSkillData{
				CLIVersion:   a.buildInfo.Version,
				SkillVersion: bundledskill.Version,
				Text:         text,
			}, strings.TrimSuffix(text, "\n"))
		},
	}
}
