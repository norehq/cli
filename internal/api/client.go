package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Error struct {
	Body    any
	Code    string
	Message string
	Status  int
}

func (e *Error) Error() string {
	return fmt.Sprintf("Nore API request failed (%d): %s", e.Status, e.Message)
}

type Client struct {
	HTTPClient *http.Client
	Registry   string
	Version    string
}

type AuthorizationRequest struct {
	AuthorizationURL string    `json:"authorizationUrl"`
	ExpiresAt        time.Time `json:"expiresAt"`
	RequestID        string    `json:"requestId"`
}

type OAuthTokenResponse struct {
	AccessToken      string    `json:"accessToken"`
	ExpiresIn        int       `json:"expiresIn"`
	RefreshExpiresAt time.Time `json:"refreshExpiresAt"`
	RefreshToken     string    `json:"refreshToken"`
	TokenType        string    `json:"tokenType"`
}

func (c Client) StartAuthorization(
	ctx context.Context,
	clientID string,
	challenge string,
	redirectURI string,
	state string,
) (AuthorizationRequest, error) {
	var output AuthorizationRequest
	err := c.Do(ctx, http.MethodPost, "/v1/cli-auth/requests", "", map[string]any{
		"clientId":      clientID,
		"codeChallenge": challenge,
		"redirectUri":   redirectURI,
		"state":         state,
	}, &output)
	return output, err
}

func (c Client) ExchangeAuthorization(
	ctx context.Context,
	code string,
	verifier string,
) (OAuthTokenResponse, error) {
	var output OAuthTokenResponse
	err := c.Do(ctx, http.MethodPost, "/v1/cli-auth/token", "", map[string]any{
		"code":         code,
		"codeVerifier": verifier,
		"grantType":    "authorization_code",
	}, &output)
	return output, err
}

func (c Client) Refresh(
	ctx context.Context,
	refreshToken string,
) (OAuthTokenResponse, error) {
	var output OAuthTokenResponse
	err := c.Do(ctx, http.MethodPost, "/v1/cli-auth/token", "", map[string]any{
		"grantType":    "refresh_token",
		"refreshToken": refreshToken,
	}, &output)
	return output, err
}

func (c Client) RevokeCurrentCredential(ctx context.Context, token string) error {
	return c.Do(ctx, http.MethodDelete, "/v1/cli/self/credential", token, nil, nil)
}

func (c Client) Do(
	ctx context.Context,
	method string,
	path string,
	token string,
	input any,
	output any,
) error {
	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(
		ctx,
		method,
		strings.TrimRight(c.Registry, "/")+path,
		body,
	)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", c.userAgent())
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return responseError(response, payload)
	}
	if output == nil || len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, output)
}

func (c Client) userAgent() string {
	version := strings.TrimSpace(c.Version)
	if version == "" || version == "dev" {
		return "nore-cli"
	}
	return "nore-cli/" + version
}

func responseError(response *http.Response, payload []byte) error {
	var body any
	if len(payload) != 0 {
		if err := json.Unmarshal(payload, &body); err != nil {
			body = string(payload)
		}
	}
	message := response.Status
	code := "API_ERROR"
	if object, ok := body.(map[string]any); ok {
		if value, ok := object["message"].(string); ok && value != "" {
			message = value
		}
		if value, ok := object["code"].(string); ok && value != "" {
			code = value
		}
		if values, ok := object["message"].([]any); ok {
			messages := make([]string, 0, len(values))
			for _, value := range values {
				messages = append(messages, fmt.Sprint(value))
			}
			message = strings.Join(messages, ", ")
		}
	}
	if value, ok := body.(string); ok && value != "" {
		message = value
	}
	return &Error{Body: body, Code: code, Message: message, Status: response.StatusCode}
}
