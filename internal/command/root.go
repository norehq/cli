package command

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/norehq/cli/internal/api"
	"github.com/norehq/cli/internal/buildinfo"
	"github.com/norehq/cli/internal/config"
	"github.com/norehq/cli/internal/output"
	"github.com/norehq/cli/internal/store"
	"github.com/spf13/cobra"
)

const (
	commandTimeout       = 30 * time.Second
	authorizationTimeout = 10 * time.Minute
	refreshBuffer        = 30 * time.Second
)

type app struct {
	buildInfo       buildinfo.Info
	configStore     config.Store
	credentialStore store.Store
	exitCode        int
	json            bool
	noColor         bool
	showVersion     bool
	stderr          io.Writer
	stdout          io.Writer
	updater         updateService
	verbose         bool
}

type authorization struct {
	kind  string
	token string
}

func Execute(args []string, stdout, stderr io.Writer) int {
	application := &app{
		buildInfo:       buildinfo.Read(),
		configStore:     config.DefaultStore(),
		credentialStore: store.DefaultStore(),
		json:            containsEnabledFlag(args, "json"),
		noColor:         noColorByDefault() || containsEnabledFlag(args, "no-color"),
		stderr:          stderr,
		stdout:          stdout,
		updater:         newGitHubUpdateService(),
		verbose:         containsEnabledFlag(args, "verbose"),
	}
	root := application.rootCommand()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		view, exitCode := errorView(err)
		application.printer().Failure(view)
		return exitCode
	}
	return application.exitCode
}

func (a *app) rootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "nore [command]",
		Short:         "Manage Nore sites and releases",
		Long:          "Nore CLI manages sites, posts, releases, and authorization from a terminal or AI agent.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if a.showVersion {
				return a.printer().Success(
					map[string]any{"name": "nore", "version": a.buildInfo.Version},
					a.buildInfo.Version,
				)
			}
			if a.json {
				return a.printer().Success(helpDocument(command), "")
			}
			return command.Help()
		},
	}
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetHelpFunc(func(command *cobra.Command, _ []string) {
		if a.json {
			_ = a.printer().Success(helpDocument(command), "")
			return
		}
		_ = a.printer().Help(command, a.buildInfo.Version)
	})
	root.PersistentFlags().BoolVarP(&a.json, "json", "j", a.json, "return JSON")
	root.PersistentFlags().BoolVar(&a.noColor, "no-color", a.noColor, "disable ANSI colors")
	root.PersistentFlags().BoolVar(&a.verbose, "verbose", a.verbose, "show verbose diagnostics")
	root.Flags().BoolVarP(&a.showVersion, "version", "v", false, "print version")
	root.PersistentFlags().SortFlags = false
	root.Flags().SortFlags = false
	root.AddCommand(
		a.loginCommand(),
		a.logoutCommand(),
		a.whoamiCommand(),
		a.configCommand(),
		a.skillCommand(),
		a.siteCommand(),
		a.postCommand(),
		a.releaseCommand(),
		a.updateCommand(),
	)
	return root
}

func helpDocument(command *cobra.Command) map[string]any {
	topic := "root"
	if command.Parent() != nil {
		topic = strings.TrimPrefix(command.CommandPath(), "nore ")
		topic = strings.ReplaceAll(topic, " ", ".")
	}
	subcommands := make([]map[string]string, 0)
	for _, child := range command.Commands() {
		if !child.IsAvailableCommand() || child.Hidden {
			continue
		}
		subcommands = append(subcommands, map[string]string{
			"description": child.Short,
			"name":        child.Name(),
		})
	}
	description := command.Long
	if description == "" {
		description = command.Short
	}
	return map[string]any{
		"command":     topic,
		"description": description,
		"subcommands": subcommands,
		"usage":       command.UseLine(),
	}
}

func (a *app) printer() output.Printer {
	return output.Printer{
		JSON:    a.json,
		NoColor: a.noColor,
		Stderr:  a.stderr,
		Stdout:  a.stdout,
		Verbose: a.verbose,
	}
}

func (a *app) client(registry string) api.Client {
	return api.Client{
		HTTPClient: &http.Client{Timeout: commandTimeout},
		Registry:   registry,
		Version:    a.buildInfo.Version,
	}
}

func (a *app) registry() (string, error) {
	values, err := a.configStore.Load()
	if err != nil {
		return "", newCommandError("CONFIG_INVALID", "The Nore CLI configuration is invalid.", err.Error())
	}
	return values.EffectiveRegistry(), nil
}

func (a *app) authenticatedRequest(
	ctx context.Context,
	method string,
	path string,
	input any,
	result any,
) error {
	registry, err := a.registry()
	if err != nil {
		return err
	}
	authorization, err := a.authorization(ctx, registry)
	if err != nil {
		return err
	}
	err = a.client(registry).Do(
		ctx,
		method,
		path,
		authorization.token,
		input,
		result,
	)
	if err == nil {
		return nil
	}
	if !isUnauthorized(err) || authorization.kind != "oauth" {
		return apiCommandError(err)
	}
	refreshed, refreshErr := a.oauthAuthorization(ctx, registry, authorization.token)
	if refreshErr != nil {
		return refreshErr
	}
	err = a.client(registry).Do(
		ctx,
		method,
		path,
		refreshed.token,
		input,
		result,
	)
	if err != nil {
		return apiCommandError(err)
	}
	return nil
}

func (a *app) authorization(ctx context.Context, registry string) (authorization, error) {
	if token := strings.TrimSpace(os.Getenv("NORE_TOKEN")); token != "" {
		return authorization{kind: "environment", token: token}, nil
	}
	credentials, err := a.credentialStore.Load()
	if errors.Is(err, store.ErrNotConfigured) {
		return authorization{}, notLoggedInError()
	}
	if err != nil {
		return authorization{}, credentialStoreError(err)
	}
	registryCredentials, err := credentials.ForRegistry(registry)
	if err != nil {
		return authorization{}, newCommandError("CREDENTIAL_INVALID", "Saved CLI credentials contain an invalid registry.", err.Error())
	}
	if token := strings.TrimSpace(registryCredentials.ManualToken); token != "" {
		return authorization{kind: "manual", token: token}, nil
	}
	if registryCredentials.OAuth == nil {
		if credentials.LegacyManualToken != "" {
			return authorization{}, legacyManualTokenError()
		}
		return authorization{}, notLoggedInError()
	}
	return a.oauthAuthorizationFromCredentials(ctx, registry, "", credentials)
}

func (a *app) oauthAuthorization(
	ctx context.Context,
	registry string,
	rejectedAccessToken string,
) (authorization, error) {
	credentials, err := a.credentialStore.Load()
	if errors.Is(err, store.ErrNotConfigured) {
		return authorization{}, notLoggedInError()
	}
	if err != nil {
		return authorization{}, credentialStoreError(err)
	}
	return a.oauthAuthorizationFromCredentials(ctx, registry, rejectedAccessToken, credentials)
}

func (a *app) oauthAuthorizationFromCredentials(
	ctx context.Context,
	registry string,
	rejectedAccessToken string,
	credentials store.Credentials,
) (authorization, error) {
	result, oauth, err := oauthAuthorizationState(credentials, registry, rejectedAccessToken)
	if err != nil {
		return authorization{}, err
	}
	if oauth == nil {
		return result, nil
	}
	return a.refreshOAuthAuthorization(ctx, registry, rejectedAccessToken)
}

func (a *app) refreshOAuthAuthorization(
	ctx context.Context,
	registry string,
	rejectedAccessToken string,
) (result authorization, err error) {
	lock, err := a.credentialStore.Lock(ctx)
	if err != nil {
		return authorization{}, credentialStoreError(err)
	}
	defer func() {
		if unlockErr := lock.Unlock(); err == nil && unlockErr != nil {
			err = credentialStoreError(unlockErr)
		}
	}()
	credentials, err := a.credentialStore.Load()
	if errors.Is(err, store.ErrNotConfigured) {
		return authorization{}, notLoggedInError()
	}
	if err != nil {
		return authorization{}, credentialStoreError(err)
	}
	result, oauth, err := oauthAuthorizationState(credentials, registry, rejectedAccessToken)
	if err != nil {
		return authorization{}, err
	}
	if oauth == nil {
		return result, nil
	}
	response, err := a.client(registry).Refresh(ctx, oauth.RefreshToken)
	if err != nil {
		return authorization{}, apiCommandError(err)
	}
	oauth.AccessExpiresAt = time.Now().Add(time.Duration(response.ExpiresIn) * time.Second).Format(time.RFC3339Nano)
	oauth.AccessToken = response.AccessToken
	oauth.RefreshExpiresAt = response.RefreshExpiresAt.Format(time.RFC3339Nano)
	oauth.RefreshToken = response.RefreshToken
	if err := a.credentialStore.Save(credentials); err != nil {
		return authorization{}, credentialStoreError(err)
	}
	return authorization{kind: "oauth", token: oauth.AccessToken}, nil
}

func oauthAuthorizationState(
	credentials store.Credentials,
	registry string,
	rejectedAccessToken string,
) (authorization, *store.OAuthCredentials, error) {
	registryCredentials, err := credentials.ForRegistry(registry)
	if err != nil {
		return authorization{}, nil, newCommandError(
			"CREDENTIAL_INVALID",
			"Saved CLI credentials contain an invalid registry.",
			err.Error(),
		)
	}
	oauth := registryCredentials.OAuth
	if oauth == nil {
		return authorization{}, nil, notLoggedInError()
	}
	if accessTokenReady(*oauth) && (rejectedAccessToken == "" || oauth.AccessToken != rejectedAccessToken) {
		return authorization{kind: "oauth", token: oauth.AccessToken}, nil, nil
	}
	refreshExpiresAt, parseErr := time.Parse(time.RFC3339Nano, oauth.RefreshExpiresAt)
	if parseErr != nil || !refreshExpiresAt.After(time.Now()) {
		return authorization{}, nil, newCommandError(
			"AUTHORIZATION_EXPIRED",
			"Your CLI authorization expired. Run \"nore login\" again.",
			nil,
		)
	}
	return authorization{}, oauth, nil
}

func accessTokenReady(credentials store.OAuthCredentials) bool {
	if credentials.AccessToken == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, credentials.AccessExpiresAt)
	return err == nil && expiresAt.After(time.Now().Add(refreshBuffer))
}

func (a *app) context(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, commandTimeout)
}

func credentialStoreError(err error) error {
	return newCommandError("CREDENTIAL_STORE_FAILED", "Saved CLI credentials could not be read or updated.", err.Error())
}

func notLoggedInError() error {
	return newCommandError(
		"NOT_LOGGED_IN",
		"No credentials are configured for the current registry. Run \"nore login\" or configure a token with \"nore config set --token <token>\".",
		nil,
	)
}

func legacyManualTokenError() error {
	return newCommandError(
		"TOKEN_REGISTRY_REQUIRED",
		"Your saved token is not associated with a registry. Run \"nore config set --token <token>\" to configure it for the current registry.",
		nil,
	)
}

func containsEnabledFlag(args []string, name string) bool {
	flag := "--" + name
	short := ""
	if name == "json" {
		short = "-j"
	}
	for _, value := range args {
		if value == flag || (short != "" && value == short) {
			return true
		}
		prefix := flag + "="
		if !strings.HasPrefix(value, prefix) {
			continue
		}
		enabled, err := strconv.ParseBool(strings.TrimPrefix(value, prefix))
		if err == nil {
			return enabled
		}
	}
	return false
}

func noColorByDefault() bool {
	_, disabled := os.LookupEnv("NO_COLOR")
	return disabled
}
