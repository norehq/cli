package command

import (
	"errors"
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
	command := &cobra.Command{Use: "get", Short: "Get a configuration value", Args: cobra.NoArgs}
	command.AddCommand(&cobra.Command{
		Use:   "registry",
		Short: "Get the effective registry",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			registry, err := a.registry()
			if err != nil {
				return err
			}
			return a.printer().Success(map[string]any{"registry": registry}, registry)
		},
	})
	return command
}

func (a *app) configSetCommand() *cobra.Command {
	command := &cobra.Command{Use: "set", Short: "Set a configuration value", Args: cobra.NoArgs}
	command.AddCommand(
		&cobra.Command{
			Use:   "token <token>",
			Short: "Save a manually created token",
			Args:  cobra.MinimumNArgs(1),
			RunE: func(command *cobra.Command, args []string) error {
				token := strings.TrimSpace(strings.Join(args, " "))
				if token == "" {
					return newCommandError("INVALID_TOKEN", "Token cannot be empty.", nil)
				}
				ctx, cancel := a.context(command.Context())
				defer cancel()
				if err := a.saveCredentials(ctx, store.Credentials{ManualToken: token}); err != nil {
					return err
				}
				return a.printer().Success(
					map[string]any{"configured": true, "source": "credentials"},
					a.printer().Done("Token saved."),
				)
			},
		},
		&cobra.Command{
			Use:   "registry <url>",
			Short: "Set the registry URL",
			Args:  cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				registry, err := a.configStore.SetRegistry(args[0])
				if err != nil {
					return newCommandError("INVALID_REGISTRY", "The registry URL is invalid.", err.Error())
				}
				return a.printer().Success(map[string]any{"registry": registry}, a.printer().Done("Registry set to "+registry))
			},
		},
	)
	return command
}

func (a *app) configUnsetCommand() *cobra.Command {
	command := &cobra.Command{Use: "unset", Short: "Unset a configuration value", Args: cobra.NoArgs}
	command.AddCommand(&cobra.Command{
		Use:   "token",
		Short: "Remove the saved manual token",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			ctx, cancel := a.context(command.Context())
			defer cancel()
			if _, err := a.credentialStore.MigrateLegacy(ctx); err != nil {
				return credentialStoreError(err)
			}
			lock, err := a.credentialStore.Lock(ctx)
			if err != nil {
				return credentialStoreError(err)
			}
			defer lock.Unlock()
			credentials, err := a.credentialStore.Load()
			if errors.Is(err, store.ErrNotConfigured) {
				return a.printer().Success(map[string]any{"configured": false}, a.printer().Done("No saved token."))
			}
			if err != nil {
				return credentialStoreError(err)
			}
			credentials.ManualToken = ""
			if err := a.credentialStore.Save(credentials); err != nil {
				return credentialStoreError(err)
			}
			return a.printer().Success(map[string]any{"configured": false}, a.printer().Done("Token removed."))
		},
	})
	return command
}

func (a *app) configResetCommand() *cobra.Command {
	command := &cobra.Command{Use: "reset", Short: "Reset a configuration value", Args: cobra.NoArgs}
	command.AddCommand(&cobra.Command{
		Use:   "registry",
		Short: "Reset the registry URL",
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
	})
	return command
}
