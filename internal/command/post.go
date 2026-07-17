package command

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func (a *app) postCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "post [command]",
		Short: "List or inspect posts",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(a.postListCommand(), a.postGetCommand())
	return command
}

func (a *app) postListCommand() *cobra.Command {
	var pageInput string
	var pageSizeInput string
	var protected bool
	var search string
	var siteInput string
	var sort string
	var sortBy string
	var status string
	command := &cobra.Command{
		Use:   "list",
		Short: "List posts",
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
			pageSize, err := positiveInteger(pageSizeInput, "page-size")
			if err != nil {
				return err
			}
			query := url.Values{}
			setQuery(query, "page", page)
			setQuery(query, "pageSize", pageSize)
			if protected {
				query.Set("protected", "true")
			}
			setQuery(query, "search", search)
			setQuery(query, "sort", sort)
			setQuery(query, "sortBy", sortBy)
			setQuery(query, "status", status)
			path := "/v1/cli/sites/" + url.PathEscape(site) + "/posts"
			if encoded := query.Encode(); encoded != "" {
				path += "?" + encoded
			}
			ctx, cancel := a.context(command.Context())
			defer cancel()
			var result map[string]any
			if err := a.authenticatedRequest(ctx, http.MethodGet, path, nil, &result); err != nil {
				return err
			}
			rows := make([][]string, 0)
			for _, value := range arrayValue(result["posts"]) {
				post := objectValue(value)
				rows = append(rows, []string{
					stringValue(post["id"]),
					stringValue(post["title"]),
					stringValue(post["status"]),
					stringValue(post["previewUrl"]),
					stringValue(post["notionUrl"]),
				})
			}
			human := a.printer().Table(
				[]string{"id", "title", "status", "previewUrl", "notionUrl"},
				rows,
			)
			human += "\n\nPage " + stringValue(result["page"]) + "/" + stringValue(result["totalPages"]) + " · " + stringValue(result["total"]) + " posts"
			return a.printer().Success(result, human)
		},
	}
	command.Flags().StringVar(&siteInput, "site", "", "site UUID or ident")
	command.Flags().StringVar(&pageInput, "page", "", "page number")
	command.Flags().StringVar(&pageSizeInput, "page-size", "", "items per page")
	command.Flags().StringVar(&search, "search", "", "search title or slug")
	command.Flags().StringVar(&status, "status", "", "comma-separated statuses")
	command.Flags().BoolVar(&protected, "protected", false, "only protected posts")
	command.Flags().StringVar(&sort, "sort", "", "sort direction")
	command.Flags().StringVar(&sortBy, "sort-by", "", "sort field")
	command.Flags().SortFlags = false
	return command
}

func (a *app) postGetCommand() *cobra.Command {
	var postInput string
	var siteInput string
	command := &cobra.Command{
		Use:   "get",
		Short: "Inspect one post",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			site, err := required(siteInput, "site")
			if err != nil {
				return err
			}
			post, err := required(postInput, "post")
			if err != nil {
				return err
			}
			ctx, cancel := a.context(command.Context())
			defer cancel()
			var result map[string]any
			path := "/v1/cli/sites/" + url.PathEscape(site) + "/posts/" + url.PathEscape(post)
			if err := a.authenticatedRequest(ctx, http.MethodGet, path, nil, &result); err != nil {
				return err
			}
			return a.printer().Success(result, a.printer().Fields([][2]string{
				{"ID", stringValue(result["id"])},
				{"Title", stringValue(result["title"])},
				{"Status", stringValue(result["status"])},
				{"Protected", stringValue(result["protected"])},
				{"Preview", stringValue(result["previewUrl"])},
				{"Notion", stringValue(result["notionUrl"])},
				{"Updated", stringValue(result["updatedAt"])},
			}))
		},
	}
	command.Flags().StringVar(&siteInput, "site", "", "site UUID or ident")
	command.Flags().StringVar(&postInput, "post", "", "post UUID")
	command.Flags().SortFlags = false
	return command
}

func setQuery(query url.Values, key string, value string) {
	if strings.TrimSpace(value) != "" {
		query.Set(key, value)
	}
}
