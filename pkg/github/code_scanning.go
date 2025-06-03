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

func GetCodeScanningAlert(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_code_scanning_alert",
			mcp.WithDescription(t("TOOL_GET_CODE_SCANNING_ALERT_DESCRIPTION", "Get details of a specific code scanning alert in a GitHub repository.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_CODE_SCANNING_ALERT_USER_TITLE", "Get code scanning alert"),
				ReadOnlyHint: toBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("The owner of the repository."),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("The name of the repository."),
			),
			mcp.WithNumber("alertNumber",
				mcp.Required(),
				mcp.Description("The number of the alert."),
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
			alertNumber, err := RequiredInt(request, "alertNumber")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			alert, resp, err := client.CodeScanning.GetAlert(ctx, owner, repo, int64(alertNumber))
			if err != nil {
				return nil, fmt.Errorf("failed to get alert: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get alert: %s", string(body))), nil
			}

			// Create simplified location structure
			type SimplifiedLocation struct {
				Path        string `json:"path,omitempty"`
				StartLine   int    `json:"start_line"`
				EndLine     int    `json:"end_line"`
				StartColumn int    `json:"start_column"`
				EndColumn   int    `json:"end_column"`
			}

			// Create simplified rule structure
			type SimplifiedRule struct {
				ID               string `json:"id,omitempty"`
				Name             string `json:"name,omitempty"`
				Severity         string `json:"severity,omitempty"`
				Description      string `json:"description,omitempty"`
				SecuritySeverity string `json:"security_severity,omitempty"`
			}

			// Create simplified tool structure
			type SimplifiedTool struct {
				Name    string `json:"name,omitempty"`
				Version string `json:"version,omitempty"`
			}

			// Create simplified alert structure
			type SimplifiedAlert struct {
				Number             int64          `json:"number"`
				CreatedAt          string         `json:"created_at,omitempty"`
				UpdatedAt          string         `json:"updated_at,omitempty"`
				HTMLURL            string         `json:"html_url,omitempty"`
				State              string         `json:"state,omitempty"`
				ClosedAt           string         `json:"closed_at,omitempty"`
				ClosedBy           string         `json:"closed_by,omitempty"`
				DismissedAt        string         `json:"dismissed_at,omitempty"`
				DismissedBy        string         `json:"dismissed_by,omitempty"`
				DismissedReason    string         `json:"dismissed_reason,omitempty"`
				Rule               SimplifiedRule `json:"rule,omitempty"`
				Tool               SimplifiedTool `json:"tool,omitempty"`
				MostRecentInstance struct {
					Ref      string             `json:"ref,omitempty"`
					State    string             `json:"state,omitempty"`
					Location SimplifiedLocation `json:"location,omitempty"`
					Message  struct {
						Text string `json:"text,omitempty"`
					} `json:"message,omitempty"`
				} `json:"most_recent_instance,omitempty"`
			}

			// Create simplified alert response
			simplifiedAlert := SimplifiedAlert{
				Number:  int64(alert.GetNumber()),
				HTMLURL: alert.GetHTMLURL(),
				State:   alert.GetState(),
			}

			// Format dates
			if alert.CreatedAt != nil {
				simplifiedAlert.CreatedAt = alert.CreatedAt.Format(time.RFC3339)
			}
			if alert.UpdatedAt != nil {
				simplifiedAlert.UpdatedAt = alert.UpdatedAt.Format(time.RFC3339)
			}
			if alert.ClosedAt != nil {
				simplifiedAlert.ClosedAt = alert.ClosedAt.Format(time.RFC3339)
			}
			if alert.DismissedAt != nil {
				simplifiedAlert.DismissedAt = alert.DismissedAt.Format(time.RFC3339)
			}

			// Add user information
			if alert.ClosedBy != nil {
				simplifiedAlert.ClosedBy = alert.ClosedBy.GetLogin()
			}
			if alert.DismissedBy != nil {
				simplifiedAlert.DismissedBy = alert.DismissedBy.GetLogin()
			}

			// Add dismissal reason
			simplifiedAlert.DismissedReason = alert.GetDismissedReason()

			// Add rule information
			if alert.Rule != nil {
				simplifiedAlert.Rule.ID = alert.Rule.GetID()
				simplifiedAlert.Rule.Name = alert.Rule.GetName()
				simplifiedAlert.Rule.Severity = alert.Rule.GetSeverity()
				simplifiedAlert.Rule.Description = alert.Rule.GetDescription()
			}

			// Add tool information
			if alert.Tool != nil {
				simplifiedAlert.Tool.Name = alert.Tool.GetName()
				simplifiedAlert.Tool.Version = alert.Tool.GetVersion()
			}

			// Add most recent instance information
			if alert.MostRecentInstance != nil {
				simplifiedAlert.MostRecentInstance.Ref = alert.MostRecentInstance.GetRef()
				simplifiedAlert.MostRecentInstance.State = alert.MostRecentInstance.GetState()

				if alert.MostRecentInstance.Location != nil {
					simplifiedAlert.MostRecentInstance.Location.Path = alert.MostRecentInstance.Location.GetPath()
					simplifiedAlert.MostRecentInstance.Location.StartLine = alert.MostRecentInstance.Location.GetStartLine()
					simplifiedAlert.MostRecentInstance.Location.EndLine = alert.MostRecentInstance.Location.GetEndLine()
					simplifiedAlert.MostRecentInstance.Location.StartColumn = alert.MostRecentInstance.Location.GetStartColumn()
					simplifiedAlert.MostRecentInstance.Location.EndColumn = alert.MostRecentInstance.Location.GetEndColumn()
				}

				if alert.MostRecentInstance.Message != nil {
					simplifiedAlert.MostRecentInstance.Message.Text = alert.MostRecentInstance.Message.GetText()
				}
			}

			r, err := json.Marshal(simplifiedAlert)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified alert: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

func ListCodeScanningAlerts(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_code_scanning_alerts",
			mcp.WithDescription(t("TOOL_LIST_CODE_SCANNING_ALERTS_DESCRIPTION", "List code scanning alerts in a GitHub repository.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_CODE_SCANNING_ALERTS_USER_TITLE", "List code scanning alerts"),
				ReadOnlyHint: toBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("The owner of the repository."),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("The name of the repository."),
			),
			mcp.WithString("ref",
				mcp.Description("The Git reference for the results you want to list."),
			),
			mcp.WithString("state",
				mcp.Description("Filter code scanning alerts by state. Defaults to open"),
				mcp.DefaultString("open"),
				mcp.Enum("open", "closed", "dismissed", "fixed"),
			),
			mcp.WithString("severity",
				mcp.Description("Filter code scanning alerts by severity"),
				mcp.Enum("critical", "high", "medium", "low", "warning", "note", "error"),
			),
			mcp.WithString("tool_name",
				mcp.Description("The name of the tool used for code scanning."),
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
			ref, err := OptionalParam[string](request, "ref")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			state, err := OptionalParam[string](request, "state")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			severity, err := OptionalParam[string](request, "severity")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			toolName, err := OptionalParam[string](request, "tool_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			alerts, resp, err := client.CodeScanning.ListAlertsForRepo(ctx, owner, repo, &github.AlertListOptions{Ref: ref, State: state, Severity: severity, ToolName: toolName})
			if err != nil {
				return nil, fmt.Errorf("failed to list alerts: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to list alerts: %s", string(body))), nil
			}

			// Create simplified location structure
			type SimplifiedLocation struct {
				Path      string `json:"path,omitempty"`
				StartLine int    `json:"start_line"`
				EndLine   int    `json:"end_line"`
			}

			// Create simplified rule structure
			type SimplifiedRule struct {
				ID          string `json:"id,omitempty"`
				Name        string `json:"name,omitempty"`
				Severity    string `json:"severity,omitempty"`
				Description string `json:"description,omitempty"`
			}

			// Create simplified tool structure
			type SimplifiedTool struct {
				Name    string `json:"name,omitempty"`
				Version string `json:"version,omitempty"`
			}

			// Create simplified alert structure
			type SimplifiedAlert struct {
				Number             int64          `json:"number"`
				CreatedAt          string         `json:"created_at,omitempty"`
				State              string         `json:"state,omitempty"`
				HTMLURL            string         `json:"html_url,omitempty"`
				Rule               SimplifiedRule `json:"rule,omitempty"`
				Tool               SimplifiedTool `json:"tool,omitempty"`
				MostRecentInstance struct {
					Location SimplifiedLocation `json:"location,omitempty"`
					Message  struct {
						Text string `json:"text,omitempty"`
					} `json:"message,omitempty"`
				} `json:"most_recent_instance,omitempty"`
			}

			// Create list of simplified alerts
			simplifiedAlerts := make([]SimplifiedAlert, 0, len(alerts))

			for _, alert := range alerts {
				simplifiedAlert := SimplifiedAlert{
					Number:  int64(alert.GetNumber()),
					State:   alert.GetState(),
					HTMLURL: alert.GetHTMLURL(),
				}

				// Format date
				if alert.CreatedAt != nil {
					simplifiedAlert.CreatedAt = alert.CreatedAt.Format(time.RFC3339)
				}

				// Add rule information
				if alert.Rule != nil {
					simplifiedAlert.Rule.ID = alert.Rule.GetID()
					simplifiedAlert.Rule.Name = alert.Rule.GetName()
					simplifiedAlert.Rule.Severity = alert.Rule.GetSeverity()
					simplifiedAlert.Rule.Description = alert.Rule.GetDescription()
				}

				// Add tool information
				if alert.Tool != nil {
					simplifiedAlert.Tool.Name = alert.Tool.GetName()
					simplifiedAlert.Tool.Version = alert.Tool.GetVersion()
				}

				// Add most recent instance information
				if alert.MostRecentInstance != nil {
					if alert.MostRecentInstance.Location != nil {
						simplifiedAlert.MostRecentInstance.Location.Path = alert.MostRecentInstance.Location.GetPath()
						simplifiedAlert.MostRecentInstance.Location.StartLine = alert.MostRecentInstance.Location.GetStartLine()
						simplifiedAlert.MostRecentInstance.Location.EndLine = alert.MostRecentInstance.Location.GetEndLine()
					}

					if alert.MostRecentInstance.Message != nil {
						simplifiedAlert.MostRecentInstance.Message.Text = alert.MostRecentInstance.Message.GetText()
					}
				}

				simplifiedAlerts = append(simplifiedAlerts, simplifiedAlert)
			}

			r, err := json.Marshal(simplifiedAlerts)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified alerts: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}
