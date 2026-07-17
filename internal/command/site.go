package command

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

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
			var result map[string]any
			if err := a.authenticatedRequest(ctx, http.MethodGet, "/v1/cli/sites", nil, &result); err != nil {
				return err
			}
			rows := make([][]string, 0)
			for _, value := range arrayValue(result["sites"]) {
				site := objectValue(value)
				rows = append(rows, []string{
					stringValue(site["id"]),
					stringValue(site["ident"]),
					stringValue(site["status"]),
					stringValue(site["previewUrl"]),
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
			site, err := required(siteInput, "site")
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
