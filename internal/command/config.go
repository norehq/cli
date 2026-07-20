package command

import (
	"strings"

	"github.com/norehq/cli/internal/config"
	"github.com/norehq/cli/internal/home"
	"github.com/norehq/cli/internal/store"
	"github.com/spf13/cobra"
)

func (a *app) configCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "config [command]",
		Short: "Manage local configuration",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		a.configShowCommand(),
		a.configPathCommand(),
		a.configGetCommand(),
		a.configSetCommand(),
		a.configUnsetCommand(),
		a.configResetCommand(),
	)
	return command
}

func (a *app) configShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show effective configuration",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			values, err := a.configStore.Load()
			if err != nil {
				return newCommandError("CONFIG_INVALID", "The Nore CLI configuration is invalid.", err.Error())
			}
			configPath, err := a.configStore.Path()
			if err != nil {
				return newCommandError("CONFIG_PATH_FAILED", "The configuration path could not be resolved.", err.Error())
			}
			credentialsPath, err := a.credentialStore.Path()
			if err != nil {
				return newCommandError("CREDENTIAL_PATH_FAILED", "The credentials path could not be resolved.", err.Error())
			}
			source := "default"
			if values.Registry != "" {
				source = "configFile"
			}
			result := map[string]any{
				"configFile":      configPath,
				"credentialsFile": credentialsPath,
				"registry":        values.EffectiveRegistry(),
				"source":          source,
			}
			return a.printer().Success(result, a.printer().Fields([][2]string{
				{"Registry", values.EffectiveRegistry()},
				{"Source", source},
				{"Config file", home.DisplayPath(configPath)},
				{"Credentials file", home.DisplayPath(credentialsPath)},
			}))
		},
	}
}

func (a *app) configPathCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the configuration file path",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			path, err := a.configStore.Path()
			if err != nil {
				return newCommandError("CONFIG_PATH_FAILED", "The configuration path could not be resolved.", err.Error())
			}
			return a.printer().Success(map[string]any{"path": path}, path)
		},
	}
}

func (a *app) configGetCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "get",
		Short: "Get configuration values",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			registry, err := a.registry()
			if err != nil {
				return err
			}
			return a.printer().Success(map[string]any{"registry": registry}, registry)
		},
	}
	command.Flags().Bool("registry", false, "get the effective registry")
	command.Flags().SortFlags = false
	command.MarkFlagsOneRequired("registry")
	return command
}

func (a *app) configSetCommand() *cobra.Command {
	var registryInput string
	var tokenInput string
	command := &cobra.Command{
		Use:   "set",
		Short: "Set configuration values",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			setRegistry := command.Flags().Changed("registry")
			setToken := command.Flags().Changed("token")
			token := strings.TrimSpace(tokenInput)
			if setToken && token == "" {
				return newCommandError("INVALID_TOKEN", "Token cannot be empty.", nil)
			}
			registry := ""
			var err error
			if setRegistry {
				registry, err = a.configStore.SetRegistry(registryInput)
				if err != nil {
					return newCommandError("INVALID_REGISTRY", "The registry URL is invalid.", err.Error())
				}
			} else {
				registry, err = a.registry()
				if err != nil {
					return err
				}
			}
			if setToken {
				ctx, cancel := a.context(command.Context())
				defer cancel()
				if err := a.updateCredentials(ctx, func(credentials *store.Credentials) error {
					registryCredentials, err := credentials.ForRegistry(registry)
					if err != nil {
						return err
					}
					credentials.LegacyManualToken = ""
					registryCredentials.ManualToken = token
					registryCredentials.OAuth = nil
					return credentials.SetRegistry(registry, registryCredentials)
				}); err != nil {
					return err
				}
			}
			result := make(map[string]any)
			messages := make([]string, 0, 2)
			if setRegistry {
				result["registry"] = registry
				messages = append(messages, a.printer().Done("Registry set to "+registry))
			}
			if setToken {
				result["configured"] = true
				result["source"] = "credentials"
				messages = append(messages, a.printer().Done("Token saved."))
			}
			return a.printer().Success(result, strings.Join(messages, "\n"))
		},
	}
	command.Flags().StringVar(&registryInput, "registry", "", "registry URL")
	command.Flags().StringVar(&tokenInput, "token", "", "manually created token")
	command.Flags().SortFlags = false
	command.MarkFlagsOneRequired("registry", "token")
	return command
}

func (a *app) configUnsetCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "unset",
		Short: "Unset configuration values",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			registry, err := a.registry()
			if err != nil {
				return err
			}
			ctx, cancel := a.context(command.Context())
			defer cancel()
			configured := false
			if err := a.updateCredentials(ctx, func(credentials *store.Credentials) error {
				registryCredentials, err := credentials.ForRegistry(registry)
				if err != nil {
					return err
				}
				configured = registryCredentials.ManualToken != "" || credentials.LegacyManualToken != ""
				credentials.LegacyManualToken = ""
				registryCredentials.ManualToken = ""
				return credentials.SetRegistry(registry, registryCredentials)
			}); err != nil {
				return err
			}
			message := a.printer().Done("No saved token.")
			if configured {
				message = a.printer().Done("Token removed.")
			}
			return a.printer().Success(map[string]any{"configured": false}, message)
		},
	}
	command.Flags().Bool("token", false, "remove the configured registry's saved token")
	command.Flags().SortFlags = false
	command.MarkFlagsOneRequired("token")
	return command
}

func (a *app) configResetCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "reset",
		Short: "Reset configuration values",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := a.configStore.ResetRegistry(); err != nil {
				return newCommandError("CONFIG_WRITE_FAILED", "The registry could not be reset.", err.Error())
			}
			return a.printer().Success(
				map[string]any{"registry": config.DefaultRegistry, "reset": true},
				a.printer().Done("Registry reset to "+config.DefaultRegistry),
			)
		},
	}
	command.Flags().Bool("registry", false, "reset the registry URL")
	command.Flags().SortFlags = false
	command.MarkFlagsOneRequired("registry")
	return command
}
