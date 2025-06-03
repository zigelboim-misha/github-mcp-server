package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	FilterDefault           = "default"
	FilterIncludeRead       = "include_read_notifications"
	FilterOnlyParticipating = "only_participating"
)

// ListNotifications creates a tool to list notifications for the current user.
func ListNotifications(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_notifications",
			mcp.WithDescription(t("TOOL_LIST_NOTIFICATIONS_DESCRIPTION", "Lists all GitHub notifications for the authenticated user, including unread notifications, mentions, review requests, assignments, and updates on issues or pull requests. Use this tool whenever the user asks what to work on next, requests a summary of their GitHub activity, wants to see pending reviews, or needs to check for new updates or tasks. This tool is the primary way to discover actionable items, reminders, and outstanding work on GitHub. Always call this tool when asked what to work on next, what is pending, or what needs attention in GitHub.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_NOTIFICATIONS_USER_TITLE", "List notifications"),
				ReadOnlyHint: toBoolPtr(true),
			}),
			mcp.WithString("filter",
				mcp.Description("Filter notifications to, use default unless specified. Read notifications are ones that have already been acknowledged by the user. Participating notifications are those that the user is directly involved in, such as issues or pull requests they have commented on or created."),
				mcp.Enum(FilterDefault, FilterIncludeRead, FilterOnlyParticipating),
			),
			mcp.WithString("since",
				mcp.Description("Only show notifications updated after the given time (ISO 8601 format)"),
			),
			mcp.WithString("before",
				mcp.Description("Only show notifications updated before the given time (ISO 8601 format)"),
			),
			mcp.WithString("owner",
				mcp.Description("Optional repository owner. If provided with repo, only notifications for this repository are listed."),
			),
			mcp.WithString("repo",
				mcp.Description("Optional repository name. If provided with owner, only notifications for this repository are listed."),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			filter, err := OptionalParam[string](request, "filter")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			since, err := OptionalParam[string](request, "since")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			before, err := OptionalParam[string](request, "before")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			owner, err := OptionalParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := OptionalParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			paginationParams, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Build options
			opts := &github.NotificationListOptions{
				All:           filter == FilterIncludeRead,
				Participating: filter == FilterOnlyParticipating,
				ListOptions: github.ListOptions{
					Page:    paginationParams.page,
					PerPage: paginationParams.perPage,
				},
			}

			// Parse time parameters if provided
			if since != "" {
				sinceTime, err := time.Parse(time.RFC3339, since)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("invalid since time format, should be RFC3339/ISO8601: %v", err)), nil
				}
				opts.Since = sinceTime
			}

			if before != "" {
				beforeTime, err := time.Parse(time.RFC3339, before)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("invalid before time format, should be RFC3339/ISO8601: %v", err)), nil
				}
				opts.Before = beforeTime
			}

			var notifications []*github.Notification
			var resp *github.Response

			if owner != "" && repo != "" {
				notifications, resp, err = client.Activity.ListRepositoryNotifications(ctx, owner, repo, opts)
			} else {
				notifications, resp, err = client.Activity.ListNotifications(ctx, opts)
			}
			if err != nil {
				return nil, fmt.Errorf("failed to get notifications: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get notifications: %s", string(body))), nil
			}

			// Create simplified repository structure
			type SimplifiedRepository struct {
				ID       int64  `json:"id"`
				Name     string `json:"name,omitempty"`
				FullName string `json:"full_name,omitempty"`
				HTMLURL  string `json:"html_url,omitempty"`
				Owner    struct {
					Login string `json:"login,omitempty"`
				} `json:"owner,omitempty"`
			}

			// Create simplified subject structure
			type SimplifiedSubject struct {
				Title            string `json:"title,omitempty"`
				URL              string `json:"url,omitempty"`
				LatestCommentURL string `json:"latest_comment_url,omitempty"`
				Type             string `json:"type,omitempty"`
			}

			// Create simplified notification structure
			type SimplifiedNotification struct {
				ID         string               `json:"id,omitempty"`
				Unread     bool                 `json:"unread"`
				Reason     string               `json:"reason,omitempty"`
				UpdatedAt  string               `json:"updated_at,omitempty"`
				LastReadAt string               `json:"last_read_at,omitempty"`
				Subject    SimplifiedSubject    `json:"subject,omitempty"`
				Repository SimplifiedRepository `json:"repository,omitempty"`
				URL        string               `json:"url,omitempty"`
			}

			// Create simplified notifications list
			simplifiedNotifications := make([]SimplifiedNotification, 0, len(notifications))

			for _, notification := range notifications {
				simplifiedNotification := SimplifiedNotification{
					ID:     notification.GetID(),
					Unread: notification.GetUnread(),
					Reason: notification.GetReason(),
					URL:    notification.GetURL(),
				}

				// Format dates
				if notification.UpdatedAt != nil {
					simplifiedNotification.UpdatedAt = notification.UpdatedAt.Format(time.RFC3339)
				}
				if notification.LastReadAt != nil {
					simplifiedNotification.LastReadAt = notification.LastReadAt.Format(time.RFC3339)
				}

				// Add subject information
				if notification.Subject != nil {
					simplifiedNotification.Subject.Title = notification.Subject.GetTitle()
					simplifiedNotification.Subject.URL = notification.Subject.GetURL()
					simplifiedNotification.Subject.LatestCommentURL = notification.Subject.GetLatestCommentURL()
					simplifiedNotification.Subject.Type = notification.Subject.GetType()
				}

				// Add repository information
				if notification.Repository != nil {
					simplifiedNotification.Repository.ID = notification.Repository.GetID()
					simplifiedNotification.Repository.Name = notification.Repository.GetName()
					simplifiedNotification.Repository.FullName = notification.Repository.GetFullName()
					simplifiedNotification.Repository.HTMLURL = notification.Repository.GetHTMLURL()

					if notification.Repository.Owner != nil {
						simplifiedNotification.Repository.Owner.Login = notification.Repository.Owner.GetLogin()
					}
				}

				simplifiedNotifications = append(simplifiedNotifications, simplifiedNotification)
			}

			// Marshal simplified response
			r, err := json.Marshal(simplifiedNotifications)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified notifications: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// DismissNotification creates a tool to mark a notification as read/done.
func DismissNotification(getclient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("dismiss_notification",
			mcp.WithDescription(t("TOOL_DISMISS_NOTIFICATION_DESCRIPTION", "Dismiss a notification by marking it as read or done")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_DISMISS_NOTIFICATION_USER_TITLE", "Dismiss notification"),
				ReadOnlyHint: toBoolPtr(false),
			}),
			mcp.WithString("threadID",
				mcp.Required(),
				mcp.Description("The ID of the notification thread"),
			),
			mcp.WithString("state", mcp.Description("The new state of the notification (read/done)"), mcp.Enum("read", "done")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getclient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			threadID, err := requiredParam[string](request, "threadID")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			state, err := requiredParam[string](request, "state")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			var resp *github.Response
			switch state {
			case "done":
				// for some inexplicable reason, the API seems to have threadID as int64 and string depending on the endpoint
				var threadIDInt int64
				threadIDInt, err = strconv.ParseInt(threadID, 10, 64)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("invalid threadID format: %v", err)), nil
				}
				resp, err = client.Activity.MarkThreadDone(ctx, threadIDInt)
			case "read":
				resp, err = client.Activity.MarkThreadRead(ctx, threadID)
			default:
				return mcp.NewToolResultError("Invalid state. Must be one of: read, done."), nil
			}

			if err != nil {
				return nil, fmt.Errorf("failed to mark notification as %s: %w", state, err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusResetContent && resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to mark notification as %s: %s", state, string(body))), nil
			}

			return mcp.NewToolResultText(fmt.Sprintf("Notification marked as %s", state)), nil
		}
}

// MarkAllNotificationsRead creates a tool to mark all notifications as read.
func MarkAllNotificationsRead(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("mark_all_notifications_read",
			mcp.WithDescription(t("TOOL_MARK_ALL_NOTIFICATIONS_READ_DESCRIPTION", "Mark all notifications as read")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_MARK_ALL_NOTIFICATIONS_READ_USER_TITLE", "Mark all notifications as read"),
				ReadOnlyHint: toBoolPtr(false),
			}),
			mcp.WithString("lastReadAt",
				mcp.Description("Describes the last point that notifications were checked (optional). Default: Now"),
			),
			mcp.WithString("owner",
				mcp.Description("Optional repository owner. If provided with repo, only notifications for this repository are marked as read."),
			),
			mcp.WithString("repo",
				mcp.Description("Optional repository name. If provided with owner, only notifications for this repository are marked as read."),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			lastReadAt, err := OptionalParam[string](request, "lastReadAt")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			owner, err := OptionalParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := OptionalParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			var lastReadTime time.Time
			if lastReadAt != "" {
				lastReadTime, err = time.Parse(time.RFC3339, lastReadAt)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("invalid lastReadAt time format, should be RFC3339/ISO8601: %v", err)), nil
				}
			} else {
				lastReadTime = time.Now()
			}

			markReadOptions := github.Timestamp{
				Time: lastReadTime,
			}

			var resp *github.Response
			if owner != "" && repo != "" {
				resp, err = client.Activity.MarkRepositoryNotificationsRead(ctx, owner, repo, markReadOptions)
			} else {
				resp, err = client.Activity.MarkNotificationsRead(ctx, markReadOptions)
			}
			if err != nil {
				return nil, fmt.Errorf("failed to mark all notifications as read: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusResetContent && resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to mark all notifications as read: %s", string(body))), nil
			}

			return mcp.NewToolResultText("All notifications marked as read"), nil
		}
}

// GetNotificationDetails creates a tool to get details for a specific notification.
func GetNotificationDetails(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_notification_details",
			mcp.WithDescription(t("TOOL_GET_NOTIFICATION_DETAILS_DESCRIPTION", "Get detailed information for a specific GitHub notification, always call this tool when the user asks for details about a specific notification, if you don't know the ID list notifications first.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_NOTIFICATION_DETAILS_USER_TITLE", "Get notification details"),
				ReadOnlyHint: toBoolPtr(true),
			}),
			mcp.WithString("notificationID",
				mcp.Required(),
				mcp.Description("The ID of the notification"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			notificationID, err := requiredParam[string](request, "notificationID")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			thread, resp, err := client.Activity.GetThread(ctx, notificationID)
			if err != nil {
				return nil, fmt.Errorf("failed to get notification details: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get notification details: %s", string(body))), nil
			}

			// Create a simplified notification thread structure
			type SimplifiedSubject struct {
				Title string `json:"title,omitempty"`
				URL   string `json:"url,omitempty"`
				Type  string `json:"type,omitempty"`
			}

			type SimplifiedRepository struct {
				Name     string `json:"name,omitempty"`
				FullName string `json:"full_name,omitempty"`
				HTMLURL  string `json:"html_url,omitempty"`
				Owner    struct {
					Login string `json:"login,omitempty"`
				} `json:"owner,omitempty"`
			}

			type SimplifiedNotificationThread struct {
				ID         string                `json:"id,omitempty"`
				Unread     bool                  `json:"unread"`
				Reason     string                `json:"reason,omitempty"`
				UpdatedAt  string                `json:"updated_at,omitempty"`
				LastReadAt string                `json:"last_read_at,omitempty"`
				URL        string                `json:"url,omitempty"`
				Subject    *SimplifiedSubject    `json:"subject,omitempty"`
				Repository *SimplifiedRepository `json:"repository,omitempty"`
			}

			// Create a simplified notification thread instance
			simplifiedThread := SimplifiedNotificationThread{
				ID:     thread.GetID(),
				Unread: thread.GetUnread(),
				Reason: thread.GetReason(),
				URL:    thread.GetURL(),
			}

			// Format dates
			if thread.UpdatedAt != nil {
				simplifiedThread.UpdatedAt = thread.UpdatedAt.Format(time.RFC3339)
			}
			if thread.LastReadAt != nil {
				simplifiedThread.LastReadAt = thread.LastReadAt.Format(time.RFC3339)
			}

			// Add subject information
			if thread.Subject != nil {
				simplifiedThread.Subject = &SimplifiedSubject{
					Title: thread.Subject.GetTitle(),
					URL:   thread.Subject.GetURL(),
					Type:  thread.Subject.GetType(),
				}
			}

			// Add repository information
			if thread.Repository != nil {
				simplifiedThread.Repository = &SimplifiedRepository{
					Name:     thread.Repository.GetName(),
					FullName: thread.Repository.GetFullName(),
					HTMLURL:  thread.Repository.GetHTMLURL(),
				}

				// Add owner information
				if thread.Repository.Owner != nil {
					simplifiedThread.Repository.Owner.Login = thread.Repository.Owner.GetLogin()
				}
			}

			r, err := json.Marshal(simplifiedThread)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified thread: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// Enum values for ManageNotificationSubscription action
const (
	NotificationActionIgnore = "ignore"
	NotificationActionWatch  = "watch"
	NotificationActionDelete = "delete"
)

// ManageNotificationSubscription creates a tool to manage a notification subscription (ignore, watch, delete)
func ManageNotificationSubscription(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("manage_notification_subscription",
			mcp.WithDescription(t("TOOL_MANAGE_NOTIFICATION_SUBSCRIPTION_DESCRIPTION", "Manage a notification subscription: ignore, watch, or delete a notification thread subscription.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_MANAGE_NOTIFICATION_SUBSCRIPTION_USER_TITLE", "Manage notification subscription"),
				ReadOnlyHint: toBoolPtr(false),
			}),
			mcp.WithString("notificationID",
				mcp.Required(),
				mcp.Description("The ID of the notification thread."),
			),
			mcp.WithString("action",
				mcp.Required(),
				mcp.Description("Action to perform: ignore, watch, or delete the notification subscription."),
				mcp.Enum(NotificationActionIgnore, NotificationActionWatch, NotificationActionDelete),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			notificationID, err := requiredParam[string](request, "notificationID")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			action, err := requiredParam[string](request, "action")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			var (
				resp   *github.Response
				result any
				apiErr error
			)

			switch action {
			case NotificationActionIgnore:
				sub := &github.Subscription{Ignored: toBoolPtr(true)}
				result, resp, apiErr = client.Activity.SetThreadSubscription(ctx, notificationID, sub)
			case NotificationActionWatch:
				sub := &github.Subscription{Ignored: toBoolPtr(false), Subscribed: toBoolPtr(true)}
				result, resp, apiErr = client.Activity.SetThreadSubscription(ctx, notificationID, sub)
			case NotificationActionDelete:
				resp, apiErr = client.Activity.DeleteThreadSubscription(ctx, notificationID)
			default:
				return mcp.NewToolResultError("Invalid action. Must be one of: ignore, watch, delete."), nil
			}

			if apiErr != nil {
				return nil, fmt.Errorf("failed to %s notification subscription: %w", action, apiErr)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				return mcp.NewToolResultError(fmt.Sprintf("failed to %s notification subscription: %s", action, string(body))), nil
			}

			if action == NotificationActionDelete {
				// Special case for delete as there is no response body
				return mcp.NewToolResultText("Notification subscription deleted"), nil
			}

			// Simplified subscription response
			if sub, ok := result.(*github.Subscription); ok {
				type SimplifiedSubscription struct {
					Subscribed *bool  `json:"subscribed,omitempty"`
					Ignored    *bool  `json:"ignored,omitempty"`
					Reason     string `json:"reason,omitempty"`
				}
				simplified := SimplifiedSubscription{
					Subscribed: sub.Subscribed,
					Ignored:    sub.Ignored,
					Reason:     sub.GetReason(),
				}
				r, err := json.Marshal(simplified)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal simplified subscription: %w", err)
				}
				return mcp.NewToolResultText(string(r)), nil
			}

			r, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}
			return mcp.NewToolResultText(string(r)), nil
		}
}

const (
	RepositorySubscriptionActionWatch  = "watch"
	RepositorySubscriptionActionIgnore = "ignore"
	RepositorySubscriptionActionDelete = "delete"
)

// ManageRepositoryNotificationSubscription creates a tool to manage a repository notification subscription (ignore, watch, delete)
func ManageRepositoryNotificationSubscription(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("manage_repository_notification_subscription",
			mcp.WithDescription(t("TOOL_MANAGE_REPOSITORY_NOTIFICATION_SUBSCRIPTION_DESCRIPTION", "Manage a repository notification subscription: ignore, watch, or delete repository notifications subscription for the provided repository.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_MANAGE_REPOSITORY_NOTIFICATION_SUBSCRIPTION_USER_TITLE", "Manage repository notification subscription"),
				ReadOnlyHint: toBoolPtr(false),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("The account owner of the repository."),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("The name of the repository."),
			),
			mcp.WithString("action",
				mcp.Required(),
				mcp.Description("Action to perform: ignore, watch, or delete the repository notification subscription."),
				mcp.Enum(RepositorySubscriptionActionIgnore, RepositorySubscriptionActionWatch, RepositorySubscriptionActionDelete),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			action, err := requiredParam[string](request, "action")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			var (
				resp   *github.Response
				result any
				apiErr error
			)

			switch action {
			case RepositorySubscriptionActionIgnore:
				sub := &github.Subscription{Ignored: toBoolPtr(true)}
				result, resp, apiErr = client.Activity.SetRepositorySubscription(ctx, owner, repo, sub)
			case RepositorySubscriptionActionWatch:
				sub := &github.Subscription{Ignored: toBoolPtr(false), Subscribed: toBoolPtr(true)}
				result, resp, apiErr = client.Activity.SetRepositorySubscription(ctx, owner, repo, sub)
			case RepositorySubscriptionActionDelete:
				resp, apiErr = client.Activity.DeleteRepositorySubscription(ctx, owner, repo)
			default:
				return mcp.NewToolResultError("Invalid action. Must be one of: ignore, watch, delete."), nil
			}

			if apiErr != nil {
				return nil, fmt.Errorf("failed to %s repository subscription: %w", action, apiErr)
			}
			if resp != nil {
				defer func() { _ = resp.Body.Close() }()
			}

			// Handle non-2xx status codes
			if resp != nil && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
				body, _ := io.ReadAll(resp.Body)
				return mcp.NewToolResultError(fmt.Sprintf("failed to %s repository subscription: %s", action, string(body))), nil
			}

			if action == RepositorySubscriptionActionDelete {
				// Special case for delete as there is no response body
				return mcp.NewToolResultText("Repository subscription deleted"), nil
			}

			// Simplified subscription response
			if sub, ok := result.(*github.Subscription); ok {
				type SimplifiedSubscription struct {
					Subscribed *bool  `json:"subscribed,omitempty"`
					Ignored    *bool  `json:"ignored,omitempty"`
					Reason     string `json:"reason,omitempty"`
				}
				simplified := SimplifiedSubscription{
					Subscribed: sub.Subscribed,
					Ignored:    sub.Ignored,
					Reason:     sub.GetReason(),
				}
				r, err := json.Marshal(simplified)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal simplified subscription: %w", err)
				}
				return mcp.NewToolResultText(string(r)), nil
			}

			r, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}
			return mcp.NewToolResultText(string(r)), nil
		}
}
