package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// SearchRepositories creates a tool to search for GitHub repositories.
func SearchRepositories(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("search_repositories",
			mcp.WithDescription(t("TOOL_SEARCH_REPOSITORIES_DESCRIPTION", "Search for GitHub repositories")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_SEARCH_REPOSITORIES_USER_TITLE", "Search repositories"),
				ReadOnlyHint: toBoolPtr(true),
			}),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Search query"),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := requiredParam[string](request, "query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pagination, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &github.SearchOptions{
				ListOptions: github.ListOptions{
					Page:    pagination.page,
					PerPage: pagination.perPage,
				},
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			result, resp, err := client.Search.Repositories(ctx, query, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to search repositories: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != 200 {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to search repositories: %s", string(body))), nil
			}

			// Create a simplified version of the response with essential fields only
			type SimplifiedOwner struct {
				Login   string `json:"login,omitempty"`
				HTMLURL string `json:"html_url,omitempty"`
			}

			type SimplifiedRepository struct {
				Name            string           `json:"name,omitempty"`
				FullName        string           `json:"full_name,omitempty"`
				Description     string           `json:"description,omitempty"`
				HTMLURL         string           `json:"html_url,omitempty"`
				Language        string           `json:"language,omitempty"`
				Private         bool             `json:"private"`
				Archived        bool             `json:"archived"`
				CreatedAt       string           `json:"created_at,omitempty"`
				UpdatedAt       string           `json:"updated_at,omitempty"`
				OpenIssuesCount int              `json:"open_issues_count"`
				Topics          []string         `json:"topics,omitempty"`
				Owner           *SimplifiedOwner `json:"owner,omitempty"`
			}

			type SimplifiedResponse struct {
				TotalCount        int                    `json:"total_count"`
				IncompleteResults bool                   `json:"incomplete_results"`
				Items             []SimplifiedRepository `json:"items"`
			}

			simplifiedResult := SimplifiedResponse{
				TotalCount:        result.GetTotal(),
				IncompleteResults: result.GetIncompleteResults(),
				Items:             make([]SimplifiedRepository, 0, len(result.Repositories)),
			}

			for _, repo := range result.Repositories {
				var ownerInfo *SimplifiedOwner
				if repo.Owner != nil {
					ownerInfo = &SimplifiedOwner{
						Login:   repo.Owner.GetLogin(),
						HTMLURL: repo.Owner.GetHTMLURL(),
					}
				}

				simplifiedRepo := SimplifiedRepository{
					Name:            repo.GetName(),
					FullName:        repo.GetFullName(),
					Description:     repo.GetDescription(),
					HTMLURL:         repo.GetHTMLURL(),
					Language:        repo.GetLanguage(),
					Private:         repo.GetPrivate(),
					Archived:        repo.GetArchived(),
					CreatedAt:       repo.GetCreatedAt().Format(time.RFC3339),
					UpdatedAt:       repo.GetUpdatedAt().Format(time.RFC3339),
					OpenIssuesCount: repo.GetOpenIssuesCount(),
					Topics:          repo.Topics,
					Owner:           ownerInfo,
				}

				simplifiedResult.Items = append(simplifiedResult.Items, simplifiedRepo)
			}

			r, err := json.Marshal(simplifiedResult)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// SearchCode creates a tool to search for code across GitHub repositories.
func SearchCode(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("search_code",
			mcp.WithDescription(t("TOOL_SEARCH_CODE_DESCRIPTION", "Search for code across GitHub repositories")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_SEARCH_CODE_USER_TITLE", "Search code"),
				ReadOnlyHint: toBoolPtr(true),
			}),
			mcp.WithString("q",
				mcp.Required(),
				mcp.Description("Search query using GitHub code search syntax"),
			),
			mcp.WithString("sort",
				mcp.Description("Sort field ('indexed' only)"),
			),
			mcp.WithString("order",
				mcp.Description("Sort order"),
				mcp.Enum("asc", "desc"),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := requiredParam[string](request, "q")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			sort, err := OptionalParam[string](request, "sort")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			order, err := OptionalParam[string](request, "order")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pagination, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &github.SearchOptions{
				Sort:  sort,
				Order: order,
				ListOptions: github.ListOptions{
					PerPage: pagination.perPage,
					Page:    pagination.page,
				},
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			result, resp, err := client.Search.Code(ctx, query, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to search code: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != 200 {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to search code: %s", string(body))), nil
			}

			// Create a simplified version of the response with essential fields only
			type SimplifiedRepository struct {
				Name     string `json:"name,omitempty"`
				FullName string `json:"full_name,omitempty"`
				HTMLURL  string `json:"html_url,omitempty"`
			}

			type SimplifiedCodeResult struct {
				Name        string               `json:"name,omitempty"`
				Path        string               `json:"path,omitempty"`
				HTMLURL     string               `json:"html_url,omitempty"`
				Repository  SimplifiedRepository `json:"repository,omitempty"`
				TextMatches []*github.TextMatch  `json:"text_matches,omitempty"`
			}

			type SimplifiedCodeResponse struct {
				TotalCount        int                    `json:"total_count"`
				IncompleteResults bool                   `json:"incomplete_results"`
				Items             []SimplifiedCodeResult `json:"items"`
			}

			simplifiedResult := SimplifiedCodeResponse{
				TotalCount:        result.GetTotal(),
				IncompleteResults: result.GetIncompleteResults(),
				Items:             make([]SimplifiedCodeResult, 0, len(result.CodeResults)),
			}

			for _, codeResult := range result.CodeResults {
				simplifiedCodeResult := SimplifiedCodeResult{
					Name:    codeResult.GetName(),
					Path:    codeResult.GetPath(),
					HTMLURL: codeResult.GetHTMLURL(),
				}

				if codeResult.Repository != nil {
					simplifiedCodeResult.Repository = SimplifiedRepository{
						Name:     codeResult.Repository.GetName(),
						FullName: codeResult.Repository.GetFullName(),
						HTMLURL:  codeResult.Repository.GetHTMLURL(),
					}
				}

				// Preserve text matches if present (important for search context)
				if codeResult.TextMatches != nil {
					simplifiedCodeResult.TextMatches = codeResult.TextMatches
				}

				simplifiedResult.Items = append(simplifiedResult.Items, simplifiedCodeResult)
			}

			r, err := json.Marshal(simplifiedResult)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// SearchUsers creates a tool to search for GitHub users.
func SearchUsers(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("search_users",
			mcp.WithDescription(t("TOOL_SEARCH_USERS_DESCRIPTION", "Search for GitHub users")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_SEARCH_USERS_USER_TITLE", "Search users"),
				ReadOnlyHint: toBoolPtr(true),
			}),
			mcp.WithString("q",
				mcp.Required(),
				mcp.Description("Search query using GitHub users search syntax"),
			),
			mcp.WithString("sort",
				mcp.Description("Sort field by category"),
				mcp.Enum("followers", "repositories", "joined"),
			),
			mcp.WithString("order",
				mcp.Description("Sort order"),
				mcp.Enum("asc", "desc"),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := requiredParam[string](request, "q")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			sort, err := OptionalParam[string](request, "sort")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			order, err := OptionalParam[string](request, "order")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pagination, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &github.SearchOptions{
				Sort:  sort,
				Order: order,
				ListOptions: github.ListOptions{
					PerPage: pagination.perPage,
					Page:    pagination.page,
				},
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			result, resp, err := client.Search.Users(ctx, query, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to search users: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != 200 {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to search users: %s", string(body))), nil
			}

			// Create a simplified version of the response with essential fields only
			type SimplifiedUser struct {
				Login       string `json:"login,omitempty"`
				ID          int64  `json:"id,omitempty"`
				NodeID      string `json:"node_id,omitempty"`
				AvatarURL   string `json:"avatar_url,omitempty"`
				HTMLURL     string `json:"html_url,omitempty"`
				Type        string `json:"type,omitempty"`
				Name        string `json:"name,omitempty"`
				Company     string `json:"company,omitempty"`
				Blog        string `json:"blog,omitempty"`
				Location    string `json:"location,omitempty"`
				Email       string `json:"email,omitempty"`
				Bio         string `json:"bio,omitempty"`
				Twitter     string `json:"twitter_username,omitempty"`
				PublicRepos int    `json:"public_repos,omitempty"`
				Followers   int    `json:"followers,omitempty"`
				Following   int    `json:"following,omitempty"`
			}

			type SimplifiedUsersResponse struct {
				TotalCount        int              `json:"total_count"`
				IncompleteResults bool             `json:"incomplete_results"`
				Items             []SimplifiedUser `json:"items"`
			}

			simplifiedResult := SimplifiedUsersResponse{
				TotalCount:        result.GetTotal(),
				IncompleteResults: result.GetIncompleteResults(),
				Items:             make([]SimplifiedUser, 0, len(result.Users)),
			}

			for _, user := range result.Users {
				simplifiedUser := SimplifiedUser{
					Login:       user.GetLogin(),
					ID:          user.GetID(),
					NodeID:      user.GetNodeID(),
					AvatarURL:   user.GetAvatarURL(),
					HTMLURL:     user.GetHTMLURL(),
					Type:        user.GetType(),
					Name:        user.GetName(),
					Company:     user.GetCompany(),
					Blog:        user.GetBlog(),
					Location:    user.GetLocation(),
					Email:       user.GetEmail(),
					Bio:         user.GetBio(),
					Twitter:     user.GetTwitterUsername(),
					PublicRepos: user.GetPublicRepos(),
					Followers:   user.GetFollowers(),
					Following:   user.GetFollowing(),
				}

				simplifiedResult.Items = append(simplifiedResult.Items, simplifiedUser)
			}

			r, err := json.Marshal(simplifiedResult)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}
