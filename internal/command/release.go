package command

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	releaseFollowTimeout = 3 * time.Minute
	releasePollInterval  = time.Second
	releaseRequestGrace  = 30 * time.Second
)

var terminalReleaseStatuses = map[string]bool{
	"FAILED":    true,
	"SKIPPED":   true,
	"SUCCEEDED": true,
}

func (a *app) releaseCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "release [command]",
		Short: "Create or inspect releases",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		a.releaseCreateCommand(),
		a.releaseListCommand(),
		a.releaseGetCommand(),
	)
	return command
}

func (a *app) releaseCreateCommand() *cobra.Command {
	var nonInteractive bool
	var siteInput string
	command := &cobra.Command{
		Use:   "create",
		Short: "Submit a release",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			site, err := required(siteInput, "site")
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(
				command.Context(),
				releaseFollowTimeout+releaseRequestGrace,
			)
			defer cancel()
			created, err := a.createRelease(ctx, site)
			if err != nil {
				return err
			}
			if nonInteractive {
				output := releaseWithOutcome(created, false)
				return a.printer().Success(output, a.releaseFields(created))
			}
			if !a.json {
				_, _ = fmt.Fprintf(
					a.stdout,
					"Release %s submitted. Following logs for up to 3 minutes.\n",
					stringValue(created["id"]),
				)
			}
			result, timedOut, err := a.followRelease(ctx, site, stringValue(created["id"]))
			if err != nil {
				return err
			}
			if timedOut && !a.json {
				_, _ = fmt.Fprintln(a.stdout, "Stopped following after 3 minutes. The release is still running.")
			}
			output := releaseWithOutcome(result, timedOut)
			output["followTimedOut"] = timedOut
			if err := a.printer().Success(output, a.releaseFields(result)); err != nil {
				return err
			}
			a.applyReleaseExitCode(result, timedOut)
			return nil
		},
	}
	command.Flags().StringVar(&siteInput, "site", "", "site UUID or ident")
	command.Flags().BoolVar(&nonInteractive, "non-interactive", false, "submit without following release logs")
	command.Flags().SortFlags = false
	return command
}

func (a *app) releaseListCommand() *cobra.Command {
	var pageInput string
	var siteInput string
	command := &cobra.Command{
		Use:   "list",
		Short: "List releases",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			site, err := required(siteInput, "site")
			if err != nil {
				return err
			}
			page, err := positiveInteger(pageInput, "page")
			if err != nil {
				return err
			}
			path := "/v1/cli/sites/" + url.PathEscape(site) + "/releases"
			if page != "" {
				path += "?page=" + url.QueryEscape(page)
			}
			ctx, cancel := a.context(command.Context())
			defer cancel()
			var result map[string]any
			if err := a.authenticatedRequest(ctx, http.MethodGet, path, nil, &result); err != nil {
				return err
			}
			releases := make([]any, 0)
			rows := make([][]string, 0)
			for _, value := range arrayValue(result["releases"]) {
				release := objectValue(value)
				withOutcome := releaseWithOutcome(release, false)
				releases = append(releases, withOutcome)
				rows = append(rows, []string{
					stringValue(withOutcome["id"]),
					stringValue(withOutcome["status"]),
					stringValue(withOutcome["outcome"]),
					stringValue(withOutcome["phase"]),
					stringValue(withOutcome["completedItems"]),
					stringValue(withOutcome["totalItems"]),
					stringValue(withOutcome["createdAt"]),
				})
			}
			result["releases"] = releases
			human := a.printer().Table(
				[]string{"id", "status", "outcome", "phase", "completedItems", "totalItems", "createdAt"},
				rows,
			)
			human += "\n\nPage " + stringValue(result["page"]) + "/" + stringValue(result["totalPages"]) + " · " + stringValue(result["total"]) + " releases"
			return a.printer().Success(result, human)
		},
	}
	command.Flags().StringVar(&siteInput, "site", "", "site UUID or ident")
	command.Flags().StringVar(&pageInput, "page", "", "page number")
	command.Flags().SortFlags = false
	return command
}

func (a *app) releaseGetCommand() *cobra.Command {
	var logs bool
	var releaseInput string
	var siteInput string
	command := &cobra.Command{
		Use:   "get",
		Short: "Inspect one release",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			site, err := required(siteInput, "site")
			if err != nil {
				return err
			}
			releaseID, err := required(releaseInput, "release")
			if err != nil {
				return err
			}
			ctx, cancel := a.context(command.Context())
			defer cancel()
			result, err := a.getRelease(ctx, site, releaseID)
			if err != nil {
				return err
			}
			output := releaseWithOutcome(result, false)
			if !logs {
				delete(output, "logs")
			}
			human := a.releaseFields(result)
			if logs {
				logLines := releaseLogLines(arrayValue(result["logs"]))
				if len(logLines) == 0 {
					human += "\n\nNo release logs."
				} else {
					human += "\n\n" + strings.Join(logLines, "\n")
				}
			}
			return a.printer().Success(output, human)
		},
	}
	command.Flags().StringVar(&siteInput, "site", "", "site UUID or ident")
	command.Flags().StringVar(&releaseInput, "release", "", "release UUID")
	command.Flags().BoolVar(&logs, "logs", false, "include release logs")
	command.Flags().SortFlags = false
	return command
}

func (a *app) createRelease(ctx context.Context, site string) (map[string]any, error) {
	var result map[string]any
	path := "/v1/cli/sites/" + url.PathEscape(site) + "/releases"
	err := a.authenticatedRequest(ctx, http.MethodPost, path, nil, &result)
	return result, err
}

func (a *app) getRelease(
	ctx context.Context,
	site string,
	releaseID string,
) (map[string]any, error) {
	var result map[string]any
	path := "/v1/cli/sites/" + url.PathEscape(site) + "/releases/" + url.PathEscape(releaseID)
	err := a.authenticatedRequest(ctx, http.MethodGet, path, nil, &result)
	return result, err
}

func (a *app) followRelease(
	ctx context.Context,
	site string,
	releaseID string,
) (map[string]any, bool, error) {
	deadline := time.Now().Add(releaseFollowTimeout)
	seenLogs := map[string]bool{}
	for {
		release, err := a.getRelease(ctx, site, releaseID)
		if err != nil {
			return nil, false, err
		}
		newLogs := make([]any, 0)
		for _, value := range arrayValue(release["logs"]) {
			log := objectValue(value)
			key := stringValue(log["source"]) + ":" + stringValue(log["id"])
			if seenLogs[key] {
				continue
			}
			seenLogs[key] = true
			newLogs = append(newLogs, log)
		}
		if !a.json {
			for _, line := range releaseLogLines(newLogs) {
				_, _ = fmt.Fprintln(a.stdout, a.printer().Info(line))
			}
		}
		if terminalReleaseStatuses[stringValue(release["status"])] {
			return release, false, nil
		}
		if !time.Now().Before(deadline) {
			return release, true, nil
		}
		timer := time.NewTimer(releasePollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, false, newCommandError("RELEASE_FOLLOW_FAILED", "Release following was interrupted.", ctx.Err().Error())
		case <-timer.C:
		}
	}
}

func releaseOutcome(release map[string]any, timedOut bool) string {
	if timedOut {
		return "INDETERMINATE"
	}
	status := stringValue(release["status"])
	switch status {
	case "SUCCEEDED":
		return "SUCCEEDED"
	case "FAILED":
		return "FAILED"
	case "SKIPPED":
		if stringValue(release["errorMessage"]) != "" || stringValue(release["failureReason"]) != "" {
			return "FAILED"
		}
		return "NO_CHANGES"
	default:
		return "IN_PROGRESS"
	}
}

func releaseWithOutcome(release map[string]any, timedOut bool) map[string]any {
	output := cloneObject(release)
	output["outcome"] = releaseOutcome(release, timedOut)
	return output
}

func (a *app) applyReleaseExitCode(release map[string]any, timedOut bool) {
	if timedOut {
		a.exitCode = 2
		return
	}
	if releaseOutcome(release, false) == "FAILED" {
		a.exitCode = 1
	}
}

func (a *app) releaseFields(release map[string]any) string {
	errorMessage := stringValue(release["errorMessage"])
	if errorMessage == "" {
		errorMessage = stringValue(release["failureReason"])
	}
	return a.printer().Fields([][2]string{
		{"ID", stringValue(release["id"])},
		{"Status", stringValue(release["status"])},
		{"Outcome", releaseOutcome(release, false)},
		{"Phase", stringValue(release["phase"])},
		{"Progress", stringValue(release["completedItems"]) + "/" + stringValue(release["totalItems"])},
		{"Started", stringValue(release["startedAt"])},
		{"Finished", stringValue(release["finishedAt"])},
		{"Error", errorMessage},
	})
}

func releaseLogLines(values []any) []string {
	lines := make([]string, 0, len(values))
	for _, value := range values {
		log := objectValue(value)
		createdAt := stringValue(log["createdAt"])
		if parsed, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
			createdAt = parsed.UTC().Format(time.RFC3339Nano)
		}
		lines = append(lines, fmt.Sprintf(
			"%s  %-7s  %s",
			createdAt,
			stringValue(log["level"]),
			stringValue(log["message"]),
		))
	}
	return lines
}
