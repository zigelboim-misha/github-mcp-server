package github

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetMe creates a tool to get details of the authenticated user.
func GetMe(getClient GetClientFn, t translations.TranslationHelperFunc) (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.NewTool("get_me",
		mcp.WithDescription(t("TOOL_GET_ME_DESCRIPTION", "Get details of the authenticated GitHub user. Use this when a request includes \"me\", \"my\". The output will not change unless the user changes their profile, so only call this once.")),
		mcp.WithToolAnnotation(mcp.ToolAnnotation{
			Title:        t("TOOL_GET_ME_USER_TITLE", "Get my user profile"),
			ReadOnlyHint: toBoolPtr(true),
		}),
		mcp.WithString("reason",
			mcp.Description("Optional: the reason for requesting the user information"),
		),
	)

	type args struct{}
	handler := mcp.NewTypedToolHandler(func(ctx context.Context, _ mcp.CallToolRequest, _ args) (*mcp.CallToolResult, error) {
		client, err := getClient(ctx)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to get GitHub client", err), nil
		}

		user, _, err := client.Users.Get(ctx, "")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to get user", err), nil
		}

		// Create simplified user structure
		type SimplifiedUser struct {
			Login     string `json:"login,omitempty"`
			HTMLURL   string `json:"html_url,omitempty"`
			Name      string `json:"name,omitempty"`
			Email     string `json:"email,omitempty"`
			Bio       string `json:"bio,omitempty"`
			Company   string `json:"company,omitempty"`
			Location  string `json:"location,omitempty"`
			Blog      string `json:"blog,omitempty"`
			AvatarURL string `json:"avatar_url,omitempty"`
			CreatedAt string `json:"created_at,omitempty"`
			UpdatedAt string `json:"updated_at,omitempty"`
		}

		// Create simplified user instance
		simplifiedUser := SimplifiedUser{
			Login:     user.GetLogin(),
			HTMLURL:   user.GetHTMLURL(),
			Name:      user.GetName(),
			Email:     user.GetEmail(),
			Bio:       user.GetBio(),
			Company:   user.GetCompany(),
			Location:  user.GetLocation(),
			Blog:      user.GetBlog(),
			AvatarURL: user.GetAvatarURL(),
		}

		// Format dates
		if user.CreatedAt != nil {
			simplifiedUser.CreatedAt = user.CreatedAt.Format(time.RFC3339)
		}
		if user.UpdatedAt != nil {
			simplifiedUser.UpdatedAt = user.UpdatedAt.Format(time.RFC3339)
		}

		r, err := json.Marshal(simplifiedUser)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal simplified user: %w", err)
		}

		return mcp.NewToolResultText(string(r)), nil
	})

	return tool, handler
}
