package command

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"charm.land/huh/v2"
	"github.com/norehq/cli/internal/browser"
	"github.com/norehq/cli/internal/config"
	"github.com/norehq/cli/internal/store"
	"github.com/spf13/cobra"
)

const (
	cliClientID   = "nore-cli"
	stateBytes    = 24
	verifierBytes = 48
)

type authorizationCallback struct {
	code  string
	err   error
	state string
}

type callbackServer struct {
	listener net.Listener
	result   chan authorizationCallback
	server   *http.Server
	stopOnce sync.Once
}

func (a *app) loginCommand() *cobra.Command {
	var registryInput string
	command := &cobra.Command{
		Use:   "login",
		Short: "Sign in through the browser",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			registry, err := a.registry()
			if err != nil {
				return err
			}
			if command.Flags().Changed("registry") {
				registry, err = a.configStore.SetRegistry(registryInput)
				if err != nil {
					return newCommandError("INVALID_REGISTRY", "The registry URL is invalid.", err.Error())
				}
			}
			ctx, cancel := context.WithTimeout(command.Context(), authorizationTimeout)
			defer cancel()
			callback, err := startCallbackServer()
			if err != nil {
				return newCommandError(
					"CALLBACK_SERVER_FAILED",
					"Nore CLI could not start the local browser callback.",
					err.Error(),
				)
			}
			defer callback.Close()
			state, err := randomValue(stateBytes)
			if err != nil {
				return newCommandError("STATE_GENERATION_FAILED", "Login state could not be generated.", err.Error())
			}
			verifier, err := randomValue(verifierBytes)
			if err != nil {
				return newCommandError("VERIFIER_GENERATION_FAILED", "Login verifier could not be generated.", err.Error())
			}
			challengeSum := sha256.Sum256([]byte(verifier))
			challenge := base64.RawURLEncoding.EncodeToString(challengeSum[:])
			request, err := a.client(registry).StartAuthorization(
				ctx,
				cliClientID,
				challenge,
				callback.RedirectURI(),
				state,
			)
			if err != nil {
				return apiCommandError(err)
			}
			browserErr := browser.Open(request.AuthorizationURL)
			a.printer().Progress(
				map[string]any{
					"authorizationUrl": request.AuthorizationURL,
					"browserOpened":    browserErr == nil,
					"event":            "authorization_required",
				},
				loginProgressMessage(request.AuthorizationURL, browserErr == nil),
			)
			var callbackResult authorizationCallback
			select {
			case callbackResult = <-callback.result:
			case <-ctx.Done():
				return newCommandError("LOGIN_TIMEOUT", "Browser authorization timed out. Run \"nore login\" again.", nil)
			}
			if callbackResult.err != nil {
				return callbackResult.err
			}
			if callbackResult.state != state {
				return newCommandError(
					"LOGIN_STATE_MISMATCH",
					"The browser authorization response could not be verified. Run \"nore login\" again.",
					nil,
				)
			}
			tokens, err := a.client(registry).ExchangeAuthorization(ctx, callbackResult.code, verifier)
			if err != nil {
				return apiCommandError(err)
			}
			credentials := store.Credentials{OAuth: &store.OAuthCredentials{
				AccessExpiresAt:  time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).Format(time.RFC3339Nano),
				AccessToken:      tokens.AccessToken,
				RefreshExpiresAt: tokens.RefreshExpiresAt.Format(time.RFC3339Nano),
				RefreshToken:     tokens.RefreshToken,
				Registry:         registry,
			}}
			if err := a.saveCredentials(ctx, credentials); err != nil {
				return err
			}
			result := map[string]any{
				"expiresAt": tokens.RefreshExpiresAt.Format(time.RFC3339Nano),
				"storage":   "file",
			}
			return a.printer().Success(result, a.printer().Fields([][2]string{
				{"Signed in", "yes"},
				{"Expires at", stringValue(result["expiresAt"])},
				{"Storage", "file"},
			}))
		},
	}
	command.Flags().StringVar(&registryInput, "registry", config.DefaultRegistry, "registry URL for this login")
	command.Flags().SortFlags = false
	return command
}

func (a *app) logoutCommand() *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:   "logout",
		Short: "Revoke the browser login and clear saved credentials",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			ctx, cancel := a.context(command.Context())
			defer cancel()
			if _, err := a.credentialStore.MigrateLegacy(ctx); err != nil {
				return credentialStoreError(err)
			}
			credentials, err := a.credentialStore.Load()
			if errors.Is(err, store.ErrNotConfigured) || (err == nil && credentials.OAuth == nil) {
				return notLoggedInError()
			}
			if err != nil {
				return credentialStoreError(err)
			}
			if !a.json && !yes {
				confirmed := false
				if err := huh.NewConfirm().
					Title("Revoke this Nore CLI login?").
					Affirmative("Revoke").
					Negative("Cancel").
					Value(&confirmed).
					Run(); err != nil {
					return newCommandError("PROMPT_FAILED", "The logout confirmation could not be shown.", err.Error())
				}
				if !confirmed {
					return a.printer().Success(
						map[string]any{"revoked": false},
						a.printer().Info("Canceled. No credentials were changed."),
					)
				}
			}
			remoteRevoked := false
			var remoteIssue any
			auth, authErr := a.oauthAuthorization(ctx, "")
			if authErr == nil {
				revokeErr := a.client(auth.registry).RevokeCurrentCredential(ctx, auth.token)
				if isUnauthorized(revokeErr) {
					refreshed, refreshErr := a.oauthAuthorization(ctx, auth.token)
					if refreshErr == nil {
						revokeErr = a.client(refreshed.registry).RevokeCurrentCredential(ctx, refreshed.token)
					} else {
						revokeErr = refreshErr
					}
				}
				remoteRevoked = revokeErr == nil
				if revokeErr != nil {
					if _, ok := revokeErr.(*commandError); !ok {
						revokeErr = apiCommandError(revokeErr)
					}
					view, _ := errorView(revokeErr)
					remoteIssue = view
				}
			} else {
				view, _ := errorView(authErr)
				remoteIssue = view
			}
			if err := a.clearOAuth(ctx); err != nil {
				return err
			}
			result := map[string]any{
				"localCredentialDeleted": true,
				"remoteRevocationIssue":  remoteIssue,
				"remoteRevoked":          remoteRevoked,
			}
			human := a.printer().Done("Logged out")
			if remoteIssue != nil {
				human = a.printer().Warning("Local credentials were removed, but remote revocation could not be confirmed.")
			}
			return a.printer().Success(result, human)
		},
	}
	command.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	command.Flags().SortFlags = false
	return command
}

func (a *app) whoamiCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated identity",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			ctx, cancel := a.context(command.Context())
			defer cancel()
			var result map[string]any
			if err := a.authenticatedRequest(ctx, http.MethodGet, "/v1/cli/self", nil, &result); err != nil {
				return err
			}
			user := objectValue(result["user"])
			credential := objectValue(result["credential"])
			membership := objectValue(user["membership"])
			name := stringValue(user["name"])
			if name == "" {
				name = stringValue(user["email"])
			}
			return a.printer().Success(result, a.printer().Fields([][2]string{
				{"User", name},
				{"Email", stringValue(user["email"])},
				{"Membership", membershipLabel(stringValue(membership["level"]))},
				{"Credential", fmt.Sprintf("%s (%s)", stringValue(credential["name"]), stringValue(credential["kind"]))},
				{"Expires at", stringValue(credential["expiresAt"])},
				{"Sites", fmt.Sprint(len(arrayValue(credential["blogIds"])))},
				{"Scopes", stringValue(credential["scopes"])},
			}))
		},
	}
}

func membershipLabel(level string) string {
	switch level {
	case "", "HOBBY":
		return "Hobby"
	case "PRESET_PRO", "PRO":
		return "Pro"
	case "TRIAL_PRO":
		return "Trial Pro"
	default:
		return level
	}
}

func (a *app) saveCredentials(ctx context.Context, credentials store.Credentials) (err error) {
	if _, migrationErr := a.credentialStore.MigrateLegacy(ctx); migrationErr != nil {
		return credentialStoreError(migrationErr)
	}
	lock, err := a.credentialStore.Lock(ctx)
	if err != nil {
		return credentialStoreError(err)
	}
	defer func() {
		if unlockErr := lock.Unlock(); err == nil && unlockErr != nil {
			err = credentialStoreError(unlockErr)
		}
	}()
	if err := a.credentialStore.Save(credentials); err != nil {
		return credentialStoreError(err)
	}
	return nil
}

func (a *app) clearOAuth(ctx context.Context) (err error) {
	lock, err := a.credentialStore.Lock(ctx)
	if err != nil {
		return credentialStoreError(err)
	}
	defer func() {
		if unlockErr := lock.Unlock(); err == nil && unlockErr != nil {
			err = credentialStoreError(unlockErr)
		}
	}()
	credentials, err := a.credentialStore.Load()
	if errors.Is(err, store.ErrNotConfigured) {
		return nil
	}
	if err != nil {
		return credentialStoreError(err)
	}
	credentials.OAuth = nil
	if err := a.credentialStore.Save(credentials); err != nil {
		return credentialStoreError(err)
	}
	return nil
}

func startCallbackServer() (*callbackServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	callback := &callbackServer{
		listener: listener,
		result:   make(chan authorizationCallback, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", callback.handle)
	callback.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		_ = callback.server.Serve(listener)
	}()
	return callback, nil
}

func (s *callbackServer) RedirectURI() string {
	return "http://" + s.listener.Addr().String() + "/callback"
}

func (s *callbackServer) Close() {
	s.stopOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.server.Shutdown(ctx)
	})
}

func (s *callbackServer) handle(response http.ResponseWriter, request *http.Request) {
	code := request.URL.Query().Get("code")
	state := request.URL.Query().Get("state")
	errorCode := request.URL.Query().Get("error")
	if code == "" || state == "" || errorCode != "" {
		response.Header().Set("Content-Type", "text/plain; charset=utf-8")
		response.WriteHeader(http.StatusBadRequest)
		_, _ = response.Write([]byte("Nore CLI authorization failed. You can close this window."))
		err := newCommandError("LOGIN_REJECTED", "Browser authorization was not completed. Run \"nore login\" again.", nil)
		if errorCode == "" {
			err = newCommandError("LOGIN_INCOMPLETE", "The browser returned an incomplete authorization response. Run \"nore login\" again.", nil)
		}
		s.deliver(authorizationCallback{err: err})
		return
	}
	response.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = response.Write([]byte("<!doctype html><meta charset=\"utf-8\"><title>Nore CLI</title><p>Authorization complete. You can close this window.</p>"))
	s.deliver(authorizationCallback{code: code, state: state})
}

func (s *callbackServer) deliver(result authorizationCallback) {
	select {
	case s.result <- result:
	default:
	}
}

func randomValue(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func loginProgressMessage(url string, opened bool) string {
	if opened {
		return "Complete authorization in your browser, or open:\n" + url
	}
	return "Open this URL to authorize Nore CLI:\n" + url
}
