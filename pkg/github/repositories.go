package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func GetCommit(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_commit",
			mcp.WithDescription(t("TOOL_GET_COMMITS_DESCRIPTION", "Get details for a commit from a GitHub repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_COMMITS_USER_TITLE", "Get commit details"),
				ReadOnlyHint: toBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("sha",
				mcp.Required(),
				mcp.Description("Commit SHA, branch name, or tag name"),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			sha, err := requiredParam[string](request, "sha")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pagination, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &github.ListOptions{
				Page:    pagination.page,
				PerPage: pagination.perPage,
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			commit, resp, err := client.Repositories.GetCommit(ctx, owner, repo, sha, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to get commit: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != 200 {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get commit: %s", string(body))), nil
			}

			// Create simplified author and committer structure
			type SimplifiedUser struct {
				Login     string `json:"login,omitempty"`
				AvatarURL string `json:"avatar_url,omitempty"`
				HTMLURL   string `json:"html_url,omitempty"`
			}

			type SimplifiedCommitDetails struct {
				Author struct {
					Name  string `json:"name,omitempty"`
					Email string `json:"email,omitempty"`
					Date  string `json:"date,omitempty"`
				} `json:"author,omitempty"`
				Committer struct {
					Name  string `json:"name,omitempty"`
					Email string `json:"email,omitempty"`
					Date  string `json:"date,omitempty"`
				} `json:"committer,omitempty"`
				Message string `json:"message,omitempty"`
			}

			type SimplifiedFile struct {
				SHA       string `json:"sha,omitempty"`
				Filename  string `json:"filename,omitempty"`
				Status    string `json:"status,omitempty"`
				Additions int    `json:"additions"`
				Deletions int    `json:"deletions"`
				Changes   int    `json:"changes"`
			}

			type SimplifiedCommit struct {
				SHA       string                  `json:"sha,omitempty"`
				NodeID    string                  `json:"node_id,omitempty"`
				HTMLURL   string                  `json:"html_url,omitempty"`
				Author    *SimplifiedUser         `json:"author,omitempty"`
				Committer *SimplifiedUser         `json:"committer,omitempty"`
				Commit    SimplifiedCommitDetails `json:"commit,omitempty"`
				Files     []SimplifiedFile        `json:"files,omitempty"`
				Stats     struct {
					Additions int `json:"additions"`
					Deletions int `json:"deletions"`
					Total     int `json:"total"`
				} `json:"stats,omitempty"`
			}

			// Create a simplified commit
			commitDetails := SimplifiedCommitDetails{}
			if commit.Commit != nil {
				if commit.Commit.Author != nil {
					commitDetails.Author.Name = commit.Commit.Author.GetName()
					commitDetails.Author.Email = commit.Commit.Author.GetEmail()
					if commit.Commit.Author.Date != nil {
						commitDetails.Author.Date = commit.Commit.Author.Date.Format(time.RFC3339)
					}
				}
				if commit.Commit.Committer != nil {
					commitDetails.Committer.Name = commit.Commit.Committer.GetName()
					commitDetails.Committer.Email = commit.Commit.Committer.GetEmail()
					if commit.Commit.Committer.Date != nil {
						commitDetails.Committer.Date = commit.Commit.Committer.Date.Format(time.RFC3339)
					}
				}
				commitDetails.Message = commit.Commit.GetMessage()
			}

			// Process author
			var author *SimplifiedUser
			if commit.Author != nil {
				author = &SimplifiedUser{
					Login:     commit.Author.GetLogin(),
					AvatarURL: commit.Author.GetAvatarURL(),
					HTMLURL:   commit.Author.GetHTMLURL(),
				}
			}

			// Process committer
			var committer *SimplifiedUser
			if commit.Committer != nil {
				committer = &SimplifiedUser{
					Login:     commit.Committer.GetLogin(),
					AvatarURL: commit.Committer.GetAvatarURL(),
					HTMLURL:   commit.Committer.GetHTMLURL(),
				}
			}

			// Process files
			files := make([]SimplifiedFile, 0, len(commit.Files))
			for _, file := range commit.Files {
				files = append(files, SimplifiedFile{
					SHA:       file.GetSHA(),
					Filename:  file.GetFilename(),
					Status:    file.GetStatus(),
					Additions: file.GetAdditions(),
					Deletions: file.GetDeletions(),
					Changes:   file.GetChanges(),
				})
			}

			// Create simplified commit
			simplifiedCommit := SimplifiedCommit{
				SHA:       commit.GetSHA(),
				NodeID:    commit.GetNodeID(),
				HTMLURL:   commit.GetHTMLURL(),
				Author:    author,
				Committer: committer,
				Commit:    commitDetails,
				Files:     files,
			}

			// Add stats if available
			if commit.Stats != nil {
				simplifiedCommit.Stats.Additions = commit.Stats.GetAdditions()
				simplifiedCommit.Stats.Deletions = commit.Stats.GetDeletions()
				simplifiedCommit.Stats.Total = commit.Stats.GetTotal()
			}

			r, err := json.Marshal(simplifiedCommit)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified commit: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// ListCommits creates a tool to get commits of a branch in a repository.
func ListCommits(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_commits",
			mcp.WithDescription(t("TOOL_LIST_COMMITS_DESCRIPTION", "Get list of commits of a branch in a GitHub repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_COMMITS_USER_TITLE", "List commits"),
				ReadOnlyHint: toBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("sha",
				mcp.Description("SHA or Branch name"),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			sha, err := OptionalParam[string](request, "sha")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pagination, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &github.CommitsListOptions{
				SHA: sha,
				ListOptions: github.ListOptions{
					Page:    pagination.page,
					PerPage: pagination.perPage,
				},
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			commits, resp, err := client.Repositories.ListCommits(ctx, owner, repo, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to list commits: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != 200 {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to list commits: %s", string(body))), nil
			}

			// Create simplified commits response
			type SimplifiedUser struct {
				Login     string `json:"login,omitempty"`
				AvatarURL string `json:"avatar_url,omitempty"`
				HTMLURL   string `json:"html_url,omitempty"`
			}

			type SimplifiedCommitDetails struct {
				Author struct {
					Name  string `json:"name,omitempty"`
					Email string `json:"email,omitempty"`
					Date  string `json:"date,omitempty"`
				} `json:"author,omitempty"`
				Committer struct {
					Name  string `json:"name,omitempty"`
					Email string `json:"email,omitempty"`
					Date  string `json:"date,omitempty"`
				} `json:"committer,omitempty"`
				Message string `json:"message,omitempty"`
			}

			type SimplifiedCommitEntry struct {
				SHA       string                  `json:"sha,omitempty"`
				NodeID    string                  `json:"node_id,omitempty"`
				HTMLURL   string                  `json:"html_url,omitempty"`
				Author    *SimplifiedUser         `json:"author,omitempty"`
				Committer *SimplifiedUser         `json:"committer,omitempty"`
				Commit    SimplifiedCommitDetails `json:"commit,omitempty"`
			}

			// Create simplified commits list
			simplifiedCommits := make([]SimplifiedCommitEntry, 0, len(commits))

			for _, commit := range commits {
				// Create commit details
				commitDetails := SimplifiedCommitDetails{}
				if commit.Commit != nil {
					if commit.Commit.Author != nil {
						commitDetails.Author.Name = commit.Commit.Author.GetName()
						commitDetails.Author.Email = commit.Commit.Author.GetEmail()
						if commit.Commit.Author.Date != nil {
							commitDetails.Author.Date = commit.Commit.Author.Date.Format(time.RFC3339)
						}
					}
					if commit.Commit.Committer != nil {
						commitDetails.Committer.Name = commit.Commit.Committer.GetName()
						commitDetails.Committer.Email = commit.Commit.Committer.GetEmail()
						if commit.Commit.Committer.Date != nil {
							commitDetails.Committer.Date = commit.Commit.Committer.Date.Format(time.RFC3339)
						}
					}
					commitDetails.Message = commit.Commit.GetMessage()
				}

				// Create simplified author
				var author *SimplifiedUser
				if commit.Author != nil {
					author = &SimplifiedUser{
						Login:     commit.Author.GetLogin(),
						AvatarURL: commit.Author.GetAvatarURL(),
						HTMLURL:   commit.Author.GetHTMLURL(),
					}
				}

				// Create simplified committer
				var committer *SimplifiedUser
				if commit.Committer != nil {
					committer = &SimplifiedUser{
						Login:     commit.Committer.GetLogin(),
						AvatarURL: commit.Committer.GetAvatarURL(),
						HTMLURL:   commit.Committer.GetHTMLURL(),
					}
				}

				// Add to list
				simplifiedCommits = append(simplifiedCommits, SimplifiedCommitEntry{
					SHA:       commit.GetSHA(),
					NodeID:    commit.GetNodeID(),
					HTMLURL:   commit.GetHTMLURL(),
					Author:    author,
					Committer: committer,
					Commit:    commitDetails,
				})
			}

			r, err := json.Marshal(simplifiedCommits)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified commits: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// ListBranches creates a tool to list branches in a GitHub repository.
func ListBranches(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_branches",
			mcp.WithDescription(t("TOOL_LIST_BRANCHES_DESCRIPTION", "List branches in a GitHub repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_BRANCHES_USER_TITLE", "List branches"),
				ReadOnlyHint: toBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pagination, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &github.BranchListOptions{
				ListOptions: github.ListOptions{
					Page:    pagination.page,
					PerPage: pagination.perPage,
				},
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			branches, resp, err := client.Repositories.ListBranches(ctx, owner, repo, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to list branches: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to list branches: %s", string(body))), nil
			}

			// Create simplified branch structure
			type SimplifiedCommit struct {
				SHA     string `json:"sha,omitempty"`
				URL     string `json:"url,omitempty"`
				HTMLURL string `json:"html_url,omitempty"`
			}

			type SimplifiedBranch struct {
				Name      string            `json:"name,omitempty"`
				Protected bool              `json:"protected"`
				Commit    *SimplifiedCommit `json:"commit,omitempty"`
			}

			// Create simplified branches
			simplifiedBranches := make([]SimplifiedBranch, 0, len(branches))

			for _, branch := range branches {
				var commit *SimplifiedCommit
				if branch.Commit != nil {
					commit = &SimplifiedCommit{
						SHA:     branch.Commit.GetSHA(),
						URL:     branch.Commit.GetURL(),
						HTMLURL: branch.Commit.GetHTMLURL(),
					}
				}

				simplifiedBranch := SimplifiedBranch{
					Name:      branch.GetName(),
					Protected: branch.GetProtected(),
					Commit:    commit,
				}

				simplifiedBranches = append(simplifiedBranches, simplifiedBranch)
			}

			r, err := json.Marshal(simplifiedBranches)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified branches: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// CreateOrUpdateFile creates a tool to create or update a file in a GitHub repository.
func CreateOrUpdateFile(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("create_or_update_file",
			mcp.WithDescription(t("TOOL_CREATE_OR_UPDATE_FILE_DESCRIPTION", "Create or update a single file in a GitHub repository. If updating, you must provide the SHA of the file you want to update.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_CREATE_OR_UPDATE_FILE_USER_TITLE", "Create or update file"),
				ReadOnlyHint: toBoolPtr(false),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner (username or organization)"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path where to create/update the file"),
			),
			mcp.WithString("content",
				mcp.Required(),
				mcp.Description("Content of the file"),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description("Commit message"),
			),
			mcp.WithString("branch",
				mcp.Required(),
				mcp.Description("Branch to create/update the file in"),
			),
			mcp.WithString("sha",
				mcp.Description("SHA of file being replaced (for updates)"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			path, err := requiredParam[string](request, "path")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			content, err := requiredParam[string](request, "content")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			message, err := requiredParam[string](request, "message")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			branch, err := requiredParam[string](request, "branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// json.Marshal encodes byte arrays with base64, which is required for the API.
			contentBytes := []byte(content)

			// Create the file options
			opts := &github.RepositoryContentFileOptions{
				Message: github.Ptr(message),
				Content: contentBytes,
				Branch:  github.Ptr(branch),
			}

			// If SHA is provided, set it (for updates)
			sha, err := OptionalParam[string](request, "sha")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if sha != "" {
				opts.SHA = github.Ptr(sha)
			}

			// Create or update the file
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			fileContent, resp, err := client.Repositories.CreateFile(ctx, owner, repo, path, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to create/update file: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != 200 && resp.StatusCode != 201 {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to create/update file: %s", string(body))), nil
			}

			r, err := json.Marshal(fileContent)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// CreateRepository creates a tool to create a new GitHub repository.
func CreateRepository(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("create_repository",
			mcp.WithDescription(t("TOOL_CREATE_REPOSITORY_DESCRIPTION", "Create a new GitHub repository in your account")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_CREATE_REPOSITORY_USER_TITLE", "Create repository"),
				ReadOnlyHint: toBoolPtr(false),
			}),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("description",
				mcp.Description("Repository description"),
			),
			mcp.WithBoolean("private",
				mcp.Description("Whether repo should be private"),
			),
			mcp.WithBoolean("autoInit",
				mcp.Description("Initialize with README"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := requiredParam[string](request, "name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			description, err := OptionalParam[string](request, "description")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			private, err := OptionalParam[bool](request, "private")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			autoInit, err := OptionalParam[bool](request, "autoInit")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			repo := &github.Repository{
				Name:        github.Ptr(name),
				Description: github.Ptr(description),
				Private:     github.Ptr(private),
				AutoInit:    github.Ptr(autoInit),
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			createdRepo, resp, err := client.Repositories.Create(ctx, "", repo)
			if err != nil {
				return nil, fmt.Errorf("failed to create repository: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusCreated {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to create repository: %s", string(body))), nil
			}

			// Create simplified repository response
			type SimplifiedOwner struct {
				Login   string `json:"login,omitempty"`
				HTMLURL string `json:"html_url,omitempty"`
			}

			type SimplifiedRepository struct {
				Name          string           `json:"name,omitempty"`
				FullName      string           `json:"full_name,omitempty"`
				Description   string           `json:"description,omitempty"`
				HTMLURL       string           `json:"html_url,omitempty"`
				CloneURL      string           `json:"clone_url,omitempty"`
				GitURL        string           `json:"git_url,omitempty"`
				SSHURL        string           `json:"ssh_url,omitempty"`
				Language      string           `json:"language,omitempty"`
				Private       bool             `json:"private"`
				Fork          bool             `json:"fork"`
				Archived      bool             `json:"archived"`
				CreatedAt     string           `json:"created_at,omitempty"`
				UpdatedAt     string           `json:"updated_at,omitempty"`
				PushedAt      string           `json:"pushed_at,omitempty"`
				DefaultBranch string           `json:"default_branch,omitempty"`
				Owner         *SimplifiedOwner `json:"owner,omitempty"`
			}

			// Extract essential fields
			simplifiedRepo := SimplifiedRepository{
				Name:          createdRepo.GetName(),
				FullName:      createdRepo.GetFullName(),
				Description:   createdRepo.GetDescription(),
				HTMLURL:       createdRepo.GetHTMLURL(),
				CloneURL:      createdRepo.GetCloneURL(),
				GitURL:        createdRepo.GetGitURL(),
				SSHURL:        createdRepo.GetSSHURL(),
				Language:      createdRepo.GetLanguage(),
				Private:       createdRepo.GetPrivate(),
				Fork:          createdRepo.GetFork(),
				Archived:      createdRepo.GetArchived(),
				DefaultBranch: createdRepo.GetDefaultBranch(),
			}

			// Format dates
			if createdRepo.CreatedAt != nil {
				simplifiedRepo.CreatedAt = createdRepo.CreatedAt.Format(time.RFC3339)
			}
			if createdRepo.UpdatedAt != nil {
				simplifiedRepo.UpdatedAt = createdRepo.UpdatedAt.Format(time.RFC3339)
			}
			if createdRepo.PushedAt != nil {
				simplifiedRepo.PushedAt = createdRepo.PushedAt.Format(time.RFC3339)
			}

			// Add owner information
			if createdRepo.Owner != nil {
				simplifiedRepo.Owner = &SimplifiedOwner{
					Login:   createdRepo.Owner.GetLogin(),
					HTMLURL: createdRepo.Owner.GetHTMLURL(),
				}
			}

			r, err := json.Marshal(simplifiedRepo)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified repository: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// GetFileContents creates a tool to get the contents of a file or directory from a GitHub repository.
func GetFileContents(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_file_contents",
			mcp.WithDescription(t("TOOL_GET_FILE_CONTENTS_DESCRIPTION", "Get the contents of a file or directory from a GitHub repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_FILE_CONTENTS_USER_TITLE", "Get file or directory contents"),
				ReadOnlyHint: toBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner (username or organization)"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path to file/directory"),
			),
			mcp.WithString("branch",
				mcp.Description("Branch to get contents from"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			path, err := requiredParam[string](request, "path")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			branch, err := OptionalParam[string](request, "branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			opts := &github.RepositoryContentGetOptions{Ref: branch}
			fileContent, dirContent, resp, err := client.Repositories.GetContents(ctx, owner, repo, path, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to get file contents: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != 200 {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get file contents: %s", string(body))), nil
			}

			// Define simplified content structures
			type SimplifiedFileContent struct {
				Type        string `json:"type,omitempty"`
				Name        string `json:"name,omitempty"`
				Path        string `json:"path,omitempty"`
				SHA         string `json:"sha,omitempty"`
				Size        int    `json:"size"`
				URL         string `json:"url,omitempty"`
				HTMLURL     string `json:"html_url,omitempty"`
				GitURL      string `json:"git_url,omitempty"`
				DownloadURL string `json:"download_url,omitempty"`
				Content     string `json:"content,omitempty"`
				Encoding    string `json:"encoding,omitempty"`
			}

			type SimplifiedDirContent struct {
				Type        string `json:"type,omitempty"`
				Name        string `json:"name,omitempty"`
				Path        string `json:"path,omitempty"`
				SHA         string `json:"sha,omitempty"`
				Size        int    `json:"size"`
				URL         string `json:"url,omitempty"`
				HTMLURL     string `json:"html_url,omitempty"`
				GitURL      string `json:"git_url,omitempty"`
				DownloadURL string `json:"download_url,omitempty"`
			}

			var result interface{}

			if fileContent != nil {
				// Single file content
				content, _ := fileContent.GetContent()
				simplifiedFile := SimplifiedFileContent{
					Type:        fileContent.GetType(),
					Name:        fileContent.GetName(),
					Path:        fileContent.GetPath(),
					SHA:         fileContent.GetSHA(),
					Size:        fileContent.GetSize(),
					URL:         fileContent.GetURL(),
					HTMLURL:     fileContent.GetHTMLURL(),
					GitURL:      fileContent.GetGitURL(),
					DownloadURL: fileContent.GetDownloadURL(),
					Content:     content,
					Encoding:    fileContent.GetEncoding(),
				}
				result = simplifiedFile
			} else {
				// Directory contents
				simplifiedDirContents := make([]SimplifiedDirContent, 0, len(dirContent))
				for _, item := range dirContent {
					simplifiedItem := SimplifiedDirContent{
						Type:        item.GetType(),
						Name:        item.GetName(),
						Path:        item.GetPath(),
						SHA:         item.GetSHA(),
						Size:        item.GetSize(),
						URL:         item.GetURL(),
						HTMLURL:     item.GetHTMLURL(),
						GitURL:      item.GetGitURL(),
						DownloadURL: item.GetDownloadURL(),
					}
					simplifiedDirContents = append(simplifiedDirContents, simplifiedItem)
				}
				result = simplifiedDirContents
			}

			r, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// ForkRepository creates a tool to fork a repository.
func ForkRepository(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("fork_repository",
			mcp.WithDescription(t("TOOL_FORK_REPOSITORY_DESCRIPTION", "Fork a GitHub repository to your account or specified organization")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FORK_REPOSITORY_USER_TITLE", "Fork repository"),
				ReadOnlyHint: toBoolPtr(false),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("organization",
				mcp.Description("Organization to fork to"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			org, err := OptionalParam[string](request, "organization")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &github.RepositoryCreateForkOptions{}
			if org != "" {
				opts.Organization = org
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			forkedRepo, resp, err := client.Repositories.CreateFork(ctx, owner, repo, opts)
			if err != nil {
				// Check if it's an acceptedError. An acceptedError indicates that the update is in progress,
				// and it's not a real error.
				if resp != nil && resp.StatusCode == http.StatusAccepted && isAcceptedError(err) {
					return mcp.NewToolResultText("Fork is in progress"), nil
				}
				return nil, fmt.Errorf("failed to fork repository: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusAccepted {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to fork repository: %s", string(body))), nil
			}

			// Create simplified repository response
			type SimplifiedOwner struct {
				Login   string `json:"login,omitempty"`
				HTMLURL string `json:"html_url,omitempty"`
			}

			type SimplifiedRepository struct {
				Name          string           `json:"name,omitempty"`
				FullName      string           `json:"full_name,omitempty"`
				Description   string           `json:"description,omitempty"`
				HTMLURL       string           `json:"html_url,omitempty"`
				CloneURL      string           `json:"clone_url,omitempty"`
				GitURL        string           `json:"git_url,omitempty"`
				SSHURL        string           `json:"ssh_url,omitempty"`
				Private       bool             `json:"private"`
				Fork          bool             `json:"fork"`
				CreatedAt     string           `json:"created_at,omitempty"`
				UpdatedAt     string           `json:"updated_at,omitempty"`
				PushedAt      string           `json:"pushed_at,omitempty"`
				DefaultBranch string           `json:"default_branch,omitempty"`
				Owner         *SimplifiedOwner `json:"owner,omitempty"`
			}

			// Extract essential fields
			simplifiedRepo := SimplifiedRepository{
				Name:          forkedRepo.GetName(),
				FullName:      forkedRepo.GetFullName(),
				Description:   forkedRepo.GetDescription(),
				HTMLURL:       forkedRepo.GetHTMLURL(),
				CloneURL:      forkedRepo.GetCloneURL(),
				GitURL:        forkedRepo.GetGitURL(),
				SSHURL:        forkedRepo.GetSSHURL(),
				Private:       forkedRepo.GetPrivate(),
				Fork:          forkedRepo.GetFork(),
				DefaultBranch: forkedRepo.GetDefaultBranch(),
			}

			// Format dates
			if forkedRepo.CreatedAt != nil {
				simplifiedRepo.CreatedAt = forkedRepo.CreatedAt.Format(time.RFC3339)
			}
			if forkedRepo.UpdatedAt != nil {
				simplifiedRepo.UpdatedAt = forkedRepo.UpdatedAt.Format(time.RFC3339)
			}
			if forkedRepo.PushedAt != nil {
				simplifiedRepo.PushedAt = forkedRepo.PushedAt.Format(time.RFC3339)
			}

			// Add owner information
			if forkedRepo.Owner != nil {
				simplifiedRepo.Owner = &SimplifiedOwner{
					Login:   forkedRepo.Owner.GetLogin(),
					HTMLURL: forkedRepo.Owner.GetHTMLURL(),
				}
			}

			r, err := json.Marshal(simplifiedRepo)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified repository: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// DeleteFile creates a tool to delete a file in a GitHub repository.
// This tool uses a more roundabout way of deleting a file than just using the client.Repositories.DeleteFile.
// This is because REST file deletion endpoint (and client.Repositories.DeleteFile) don't add commit signing to the deletion commit,
// unlike how the endpoint backing the create_or_update_files tool does. This appears to be a quirk of the API.
// The approach implemented here gets automatic commit signing when used with either the github-actions user or as an app,
// both of which suit an LLM well.
func DeleteFile(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("delete_file",
			mcp.WithDescription(t("TOOL_DELETE_FILE_DESCRIPTION", "Delete a file from a GitHub repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:           t("TOOL_DELETE_FILE_USER_TITLE", "Delete file"),
				ReadOnlyHint:    toBoolPtr(false),
				DestructiveHint: toBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner (username or organization)"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path to the file to delete"),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description("Commit message"),
			),
			mcp.WithString("branch",
				mcp.Required(),
				mcp.Description("Branch to delete the file from"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			path, err := requiredParam[string](request, "path")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			message, err := requiredParam[string](request, "message")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			branch, err := requiredParam[string](request, "branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			// Get the reference for the branch
			ref, resp, err := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+branch)
			if err != nil {
				return nil, fmt.Errorf("failed to get branch reference: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			// Get the commit object that the branch points to
			baseCommit, resp, err := client.Git.GetCommit(ctx, owner, repo, *ref.Object.SHA)
			if err != nil {
				return nil, fmt.Errorf("failed to get base commit: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get commit: %s", string(body))), nil
			}

			// Create a tree entry for the file deletion by setting SHA to nil
			treeEntries := []*github.TreeEntry{
				{
					Path: github.Ptr(path),
					Mode: github.Ptr("100644"), // Regular file mode
					Type: github.Ptr("blob"),
					SHA:  nil, // Setting SHA to nil deletes the file
				},
			}

			// Create a new tree with the deletion
			newTree, resp, err := client.Git.CreateTree(ctx, owner, repo, *baseCommit.Tree.SHA, treeEntries)
			if err != nil {
				return nil, fmt.Errorf("failed to create tree: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusCreated {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to create tree: %s", string(body))), nil
			}

			// Create a new commit with the new tree
			commit := &github.Commit{
				Message: github.Ptr(message),
				Tree:    newTree,
				Parents: []*github.Commit{{SHA: baseCommit.SHA}},
			}
			newCommit, resp, err := client.Git.CreateCommit(ctx, owner, repo, commit, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create commit: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusCreated {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to create commit: %s", string(body))), nil
			}

			// Update the branch reference to point to the new commit
			ref.Object.SHA = newCommit.SHA
			_, resp, err = client.Git.UpdateRef(ctx, owner, repo, ref, false)
			if err != nil {
				return nil, fmt.Errorf("failed to update reference: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to update reference: %s", string(body))), nil
			}

			// Create a simplified response
			type SimplifiedCommit struct {
				SHA     string `json:"sha,omitempty"`
				HTMLURL string `json:"html_url,omitempty"`
				Message string `json:"message,omitempty"`
			}

			type SimplifiedDeleteResponse struct {
				Commit  SimplifiedCommit `json:"commit"`
				Content interface{}      `json:"content"`
			}

			simplifiedCommit := SimplifiedCommit{
				SHA:     newCommit.GetSHA(),
				HTMLURL: newCommit.GetHTMLURL(),
				Message: commit.GetMessage(),
			}

			simplifiedResponse := SimplifiedDeleteResponse{
				Commit:  simplifiedCommit,
				Content: nil,
			}

			r, err := json.Marshal(simplifiedResponse)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// CreateBranch creates a tool to create a new branch.
func CreateBranch(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("create_branch",
			mcp.WithDescription(t("TOOL_CREATE_BRANCH_DESCRIPTION", "Create a new branch in a GitHub repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_CREATE_BRANCH_USER_TITLE", "Create branch"),
				ReadOnlyHint: toBoolPtr(false),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("branch",
				mcp.Required(),
				mcp.Description("Name for new branch"),
			),
			mcp.WithString("from_branch",
				mcp.Description("Source branch (defaults to repo default)"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			branch, err := requiredParam[string](request, "branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			fromBranch, err := OptionalParam[string](request, "from_branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			// Get the source branch SHA
			var ref *github.Reference

			if fromBranch == "" {
				// Get default branch if from_branch not specified
				repository, resp, err := client.Repositories.Get(ctx, owner, repo)
				if err != nil {
					return nil, fmt.Errorf("failed to get repository: %w", err)
				}
				defer func() { _ = resp.Body.Close() }()

				fromBranch = *repository.DefaultBranch
			}

			// Get SHA of source branch
			ref, resp, err := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+fromBranch)
			if err != nil {
				return nil, fmt.Errorf("failed to get reference: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			// Create new branch
			newRef := &github.Reference{
				Ref:    github.Ptr("refs/heads/" + branch),
				Object: &github.GitObject{SHA: ref.Object.SHA},
			}

			createdRef, resp, err := client.Git.CreateRef(ctx, owner, repo, newRef)
			if err != nil {
				return nil, fmt.Errorf("failed to create branch: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			// Create simplified reference structure
			type SimplifiedReference struct {
				Ref string `json:"ref,omitempty"`
				URL string `json:"url,omitempty"`
				SHA string `json:"sha,omitempty"`
			}

			// Create simplified reference instance
			simplifiedRef := SimplifiedReference{
				Ref: createdRef.GetRef(),
				URL: createdRef.GetURL(),
				SHA: createdRef.Object.GetSHA(),
			}

			r, err := json.Marshal(simplifiedRef)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// PushFiles creates a tool to push multiple files in a single commit to a GitHub repository.
func PushFiles(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("push_files",
			mcp.WithDescription(t("TOOL_PUSH_FILES_DESCRIPTION", "Push multiple files to a GitHub repository in a single commit")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_PUSH_FILES_USER_TITLE", "Push files to repository"),
				ReadOnlyHint: toBoolPtr(false),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("branch",
				mcp.Required(),
				mcp.Description("Branch to push to"),
			),
			mcp.WithArray("files",
				mcp.Required(),
				mcp.Items(
					map[string]interface{}{
						"type":                 "object",
						"additionalProperties": false,
						"required":             []string{"path", "content"},
						"properties": map[string]interface{}{
							"path": map[string]interface{}{
								"type":        "string",
								"description": "path to the file",
							},
							"content": map[string]interface{}{
								"type":        "string",
								"description": "file content",
							},
						},
					}),
				mcp.Description("Array of file objects to push, each object with path (string) and content (string)"),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description("Commit message"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			branch, err := requiredParam[string](request, "branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			message, err := requiredParam[string](request, "message")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Parse files parameter - this should be an array of objects with path and content
			filesObj, ok := request.GetArguments()["files"].([]interface{})
			if !ok {
				return mcp.NewToolResultError("files parameter must be an array of objects with path and content"), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			// Get the reference for the branch
			ref, resp, err := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+branch)
			if err != nil {
				return nil, fmt.Errorf("failed to get branch reference: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			// Get the commit object that the branch points to
			baseCommit, resp, err := client.Git.GetCommit(ctx, owner, repo, *ref.Object.SHA)
			if err != nil {
				return nil, fmt.Errorf("failed to get base commit: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			// Create tree entries for all files
			var entries []*github.TreeEntry

			for _, file := range filesObj {
				fileMap, ok := file.(map[string]interface{})
				if !ok {
					return mcp.NewToolResultError("each file must be an object with path and content"), nil
				}

				path, ok := fileMap["path"].(string)
				if !ok || path == "" {
					return mcp.NewToolResultError("each file must have a path"), nil
				}

				content, ok := fileMap["content"].(string)
				if !ok {
					return mcp.NewToolResultError("each file must have content"), nil
				}

				// Create a tree entry for the file
				entries = append(entries, &github.TreeEntry{
					Path:    github.Ptr(path),
					Mode:    github.Ptr("100644"), // Regular file mode
					Type:    github.Ptr("blob"),
					Content: github.Ptr(content),
				})
			}

			// Create a new tree with the file entries
			newTree, resp, err := client.Git.CreateTree(ctx, owner, repo, *baseCommit.Tree.SHA, entries)
			if err != nil {
				return nil, fmt.Errorf("failed to create tree: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			// Create a new commit
			commit := &github.Commit{
				Message: github.Ptr(message),
				Tree:    newTree,
				Parents: []*github.Commit{{SHA: baseCommit.SHA}},
			}
			newCommit, resp, err := client.Git.CreateCommit(ctx, owner, repo, commit, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create commit: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			// Update the reference to point to the new commit
			ref.Object.SHA = newCommit.SHA
			updatedRef, resp, err := client.Git.UpdateRef(ctx, owner, repo, ref, false)
			if err != nil {
				return nil, fmt.Errorf("failed to update reference: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			// Create simplified reference structure
			type SimplifiedReference struct {
				Ref     string `json:"ref,omitempty"`
				URL     string `json:"url,omitempty"`
				SHA     string `json:"sha,omitempty"`
				Message string `json:"message,omitempty"`
			}

			// Create simplified reference instance
			simplifiedRef := SimplifiedReference{
				Ref:     updatedRef.GetRef(),
				URL:     updatedRef.GetURL(),
				SHA:     updatedRef.Object.GetSHA(),
				Message: message,
			}

			r, err := json.Marshal(simplifiedRef)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// ListTags creates a tool to list tags in a GitHub repository.
func ListTags(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_tags",
			mcp.WithDescription(t("TOOL_LIST_TAGS_DESCRIPTION", "List git tags in a GitHub repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_TAGS_USER_TITLE", "List tags"),
				ReadOnlyHint: toBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pagination, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &github.ListOptions{
				Page:    pagination.page,
				PerPage: pagination.perPage,
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			tags, resp, err := client.Repositories.ListTags(ctx, owner, repo, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to list tags: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to list tags: %s", string(body))), nil
			}

			// Create simplified tag structure
			type SimplifiedCommit struct {
				SHA     string `json:"sha,omitempty"`
				URL     string `json:"url,omitempty"`
				HTMLURL string `json:"html_url,omitempty"`
			}

			type SimplifiedTag struct {
				Name       string            `json:"name,omitempty"`
				ZipballURL string            `json:"zipball_url,omitempty"`
				TarballURL string            `json:"tarball_url,omitempty"`
				Commit     *SimplifiedCommit `json:"commit,omitempty"`
			}

			// Create simplified tags list
			simplifiedTags := make([]SimplifiedTag, 0, len(tags))

			for _, tag := range tags {
				var commit *SimplifiedCommit
				if tag.Commit != nil {
					commit = &SimplifiedCommit{
						SHA:     tag.Commit.GetSHA(),
						URL:     tag.Commit.GetURL(),
						HTMLURL: tag.Commit.GetHTMLURL(),
					}
				}

				simplifiedTag := SimplifiedTag{
					Name:       tag.GetName(),
					ZipballURL: tag.GetZipballURL(),
					TarballURL: tag.GetTarballURL(),
					Commit:     commit,
				}

				simplifiedTags = append(simplifiedTags, simplifiedTag)
			}

			r, err := json.Marshal(simplifiedTags)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified tags: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// GetTag creates a tool to get details about a specific tag in a GitHub repository.
func GetTag(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_tag",
			mcp.WithDescription(t("TOOL_GET_TAG_DESCRIPTION", "Get details about a specific git tag in a GitHub repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_TAG_USER_TITLE", "Get tag details"),
				ReadOnlyHint: toBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("tag",
				mcp.Required(),
				mcp.Description("Tag name"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			tag, err := requiredParam[string](request, "tag")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			// First get the tag reference
			ref, resp, err := client.Git.GetRef(ctx, owner, repo, "refs/tags/"+tag)
			if err != nil {
				return nil, fmt.Errorf("failed to get tag reference: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get tag reference: %s", string(body))), nil
			}

			// Then get the tag object
			tagObj, resp, err := client.Git.GetTag(ctx, owner, repo, *ref.Object.SHA)
			if err != nil {
				return nil, fmt.Errorf("failed to get tag object: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get tag object: %s", string(body))), nil
			}

			// Create simplified tag object
			type SimplifiedTagObject struct {
				Tag     string `json:"tag,omitempty"`
				SHA     string `json:"sha,omitempty"`
				URL     string `json:"url,omitempty"`
				Message string `json:"message,omitempty"`
				Tagger  struct {
					Name  string `json:"name,omitempty"`
					Email string `json:"email,omitempty"`
					Date  string `json:"date,omitempty"`
				} `json:"tagger,omitempty"`
				Object struct {
					Type string `json:"type,omitempty"`
					SHA  string `json:"sha,omitempty"`
					URL  string `json:"url,omitempty"`
				} `json:"object,omitempty"`
			}

			// Create simplified tag
			simplifiedTag := SimplifiedTagObject{
				Tag:     tagObj.GetTag(),
				SHA:     tagObj.GetSHA(),
				URL:     tagObj.GetURL(),
				Message: tagObj.GetMessage(),
			}

			// Add tagger information
			if tagObj.Tagger != nil {
				simplifiedTag.Tagger.Name = tagObj.Tagger.GetName()
				simplifiedTag.Tagger.Email = tagObj.Tagger.GetEmail()
				if tagObj.Tagger.Date != nil {
					simplifiedTag.Tagger.Date = tagObj.Tagger.Date.Format(time.RFC3339)
				}
			}

			// Add object information
			if tagObj.Object != nil {
				simplifiedTag.Object.Type = tagObj.Object.GetType()
				simplifiedTag.Object.SHA = tagObj.Object.GetSHA()
				simplifiedTag.Object.URL = tagObj.Object.GetURL()
			}

			r, err := json.Marshal(simplifiedTag)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified tag: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}
