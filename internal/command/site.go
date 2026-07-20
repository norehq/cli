package command

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"charm.land/huh/v2"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

type siteSummary struct {
	ID         string
	Ident      string
	PreviewURL string
	Status     string
}

type siteSelector func(context.Context, []siteSummary) (string, error)

func (a *app) siteCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "site [command]",
		Short: "List or inspect sites",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(a.siteListCommand(), a.siteGetCommand())
	return command
}

func (a *app) siteListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List authorized sites",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			ctx, cancel := a.context(command.Context())
			defer cancel()
			result, sites, err := a.listSites(ctx)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(sites))
			for _, site := range sites {
				rows = append(rows, []string{
					site.ID,
					site.Ident,
					site.Status,
					site.PreviewURL,
				})
			}
			return a.printer().Success(result, a.printer().Table(
				[]string{"id", "ident", "status", "previewUrl"},
				rows,
			))
		},
	}
}

func (a *app) siteGetCommand() *cobra.Command {
	var siteInput string
	command := &cobra.Command{
		Use:   "get",
		Short: "Inspect one site",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			site, err := a.resolveSite(command.Context(), siteInput, false)
			if err != nil {
				return err
			}
			ctx, cancel := a.context(command.Context())
			defer cancel()
			var result map[string]any
			path := "/v1/cli/sites/" + url.PathEscape(site)
			if err := a.authenticatedRequest(ctx, http.MethodGet, path, nil, &result); err != nil {
				return err
			}
			template := stringValue(result["templateRelease"])
			if template == "" {
				template = "—"
			}
			return a.printer().Success(result, a.printer().Fields([][2]string{
				{"ID", stringValue(result["id"])},
				{"Ident", stringValue(result["ident"])},
				{"Status", stringValue(result["status"])},
				{"Preview", stringValue(result["previewUrl"])},
				{"Notion", stringValue(result["notionUrl"])},
				{"Last synced", stringValue(result["lastSyncedAt"])},
				{"Template", template + " / " + stringValue(result["templateHealthStatus"])},
			}))
		},
	}
	command.Flags().StringVar(&siteInput, "site", "", "site UUID or ident")
	command.Flags().SortFlags = false
	return command
}

func (a *app) listSites(ctx context.Context) (map[string]any, []siteSummary, error) {
	var result map[string]any
	if err := a.authenticatedRequest(ctx, http.MethodGet, "/v1/cli/sites", nil, &result); err != nil {
		return nil, nil, err
	}
	sites := make([]siteSummary, 0)
	for _, value := range arrayValue(result["sites"]) {
		site := objectValue(value)
		sites = append(sites, siteSummary{
			ID:         stringValue(site["id"]),
			Ident:      stringValue(site["ident"]),
			PreviewURL: stringValue(site["previewUrl"]),
			Status:     stringValue(site["status"]),
		})
	}
	return result, sites, nil
}

func (a *app) resolveSite(
	ctx context.Context,
	siteInput string,
	nonInteractive bool,
) (string, error) {
	site := strings.TrimSpace(siteInput)
	if site != "" {
		return site, nil
	}
	selector := a.siteSelector
	if a.json || nonInteractive || (selector == nil && !siteSelectionAvailable(a.stderr)) {
		return "", siteSelectionRequiredError(a.json || nonInteractive)
	}
	requestCtx, cancel := a.context(ctx)
	_, sites, err := a.listSites(requestCtx)
	cancel()
	if err != nil {
		return "", err
	}
	selectable := selectableSites(sites)
	if len(selectable) == 0 {
		return "", newCommandError(
			"NO_SITES_AVAILABLE",
			"No sites are available for the current credential.",
			nil,
		)
	}
	_, _ = fmt.Fprintln(
		a.stderr,
		a.printer().Info("No --site was provided. Select a site to continue."),
	)
	if selector == nil {
		selector = a.promptForSite
	}
	selected, err := selector(ctx, selectable)
	if errors.Is(err, huh.ErrUserAborted) {
		return "", newCommandError("SITE_SELECTION_CANCELED", "Site selection was canceled.", nil)
	}
	if err != nil {
		return "", newCommandError("SITE_SELECTION_FAILED", "The site selector could not be shown.", err.Error())
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return "", newCommandError("SITE_SELECTION_FAILED", "The site selector returned no site.", nil)
	}
	return selected, nil
}

func (a *app) promptForSite(ctx context.Context, sites []siteSummary) (string, error) {
	selected := ""
	options := make([]huh.Option[string], 0, len(sites))
	for _, site := range sites {
		options = append(options, huh.NewOption(siteLabel(site), siteReference(site)))
	}
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a site").
				Options(options...).
				Value(&selected),
		),
	).
		WithInput(os.Stdin).
		WithOutput(a.stderr).
		RunWithContext(ctx)
	return selected, err
}

func selectableSites(sites []siteSummary) []siteSummary {
	selectable := make([]siteSummary, 0, len(sites))
	for _, site := range sites {
		if siteReference(site) != "" {
			selectable = append(selectable, site)
		}
	}
	return selectable
}

func siteReference(site siteSummary) string {
	if site.ID != "" {
		return site.ID
	}
	return site.Ident
}

func siteLabel(site siteSummary) string {
	ident := site.Ident
	if ident == "" {
		ident = site.ID
	}
	values := []string{ident}
	if site.Status != "" {
		values = append(values, site.Status)
	}
	if site.PreviewURL != "" {
		values = append(values, site.PreviewURL)
	}
	return strings.Join(values, " · ")
}

func siteSelectionAvailable(output any) bool {
	outputDescriptor, ok := output.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(outputDescriptor.Fd())
}

func siteSelectionRequiredError(includeAgentHint bool) error {
	if !includeAgentHint {
		return newCommandError(
			"SITE_SELECTION_REQUIRED",
			"No --site was provided. Pass \"--site <site>\" when interactive site selection is unavailable.",
			nil,
		)
	}
	return newCommandError(
		"SITE_SELECTION_REQUIRED",
		"No --site was provided. For agents or CI, run \"nore site list --json\", then pass \"--site <site>\".",
		nil,
	)
}
