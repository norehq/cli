package command

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/norehq/cli/internal/api"
	"github.com/norehq/cli/internal/output"
)

type commandError struct {
	Code     string
	Details  any
	ExitCode int
	Message  string
}

func (e *commandError) Error() string {
	return e.Message
}

func newCommandError(code string, message string, details any) error {
	return &commandError{Code: code, Details: details, ExitCode: 1, Message: message}
}

func apiCommandError(err error) error {
	var apiError *api.Error
	if errors.As(err, &apiError) {
		code := apiError.Code
		if code == "" || code == "API_ERROR" {
			code = fmt.Sprintf("HTTP_%d", apiError.Status)
		}
		return &commandError{
			Code:     code,
			Details:  apiError.Body,
			ExitCode: 1,
			Message:  apiError.Message,
		}
	}
	return newCommandError(
		"NETWORK_ERROR",
		"Nore API could not be reached.",
		nil,
	)
}

func isUnauthorized(err error) bool {
	var apiError *api.Error
	return errors.As(err, &apiError) && apiError.Status == http.StatusUnauthorized
}

func errorView(err error) (output.Error, int) {
	var commandErr *commandError
	if errors.As(err, &commandErr) {
		return output.Error{
			Code:    commandErr.Code,
			Details: commandErr.Details,
			Message: commandErr.Message,
		}, commandErr.ExitCode
	}
	return output.Error{
		Code:    "COMMAND_FAILED",
		Details: err.Error(),
		Message: err.Error(),
	}, 1
}
