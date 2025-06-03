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

func GetSecretScanningAlert(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool(
			"get_secret_scanning_alert",
			mcp.WithDescription(t("TOOL_GET_SECRET_SCANNING_ALERT_DESCRIPTION", "Get details of a specific secret scanning alert in a GitHub repository.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_SECRET_SCANNING_ALERT_USER_TITLE", "Get secret scanning alert"),
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

			alert, resp, err := client.SecretScanning.GetAlert(ctx, owner, repo, int64(alertNumber))
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

			// Create simplified alert structure
			type SimplifiedSecretScanningAlert struct {
				Number                int64  `json:"number"`
				CreatedAt             string `json:"created_at,omitempty"`
				UpdatedAt             string `json:"updated_at,omitempty"`
				HTMLURL               string `json:"html_url,omitempty"`
				State                 string `json:"state,omitempty"`
				Resolution            string `json:"resolution,omitempty"`
				ResolvedAt            string `json:"resolved_at,omitempty"`
				ResolvedBy            string `json:"resolved_by,omitempty"`
				SecretType            string `json:"secret_type,omitempty"`
				SecretTypeDisplayName string `json:"secret_type_display_name,omitempty"`
				Secret                string `json:"secret,omitempty"`
			}

			// Create simplified alert instance
			simplifiedAlert := SimplifiedSecretScanningAlert{
				Number:                int64(alert.GetNumber()),
				HTMLURL:               alert.GetHTMLURL(),
				State:                 alert.GetState(),
				Resolution:            alert.GetResolution(),
				SecretType:            alert.GetSecretType(),
				SecretTypeDisplayName: alert.GetSecretTypeDisplayName(),
				Secret:                alert.GetSecret(),
			}

			// Format dates
			if alert.CreatedAt != nil {
				simplifiedAlert.CreatedAt = alert.CreatedAt.Format(time.RFC3339)
			}
			if alert.UpdatedAt != nil {
				simplifiedAlert.UpdatedAt = alert.UpdatedAt.Format(time.RFC3339)
			}
			if alert.ResolvedAt != nil {
				simplifiedAlert.ResolvedAt = alert.ResolvedAt.Format(time.RFC3339)
			}

			// Add user information
			if alert.ResolvedBy != nil {
				simplifiedAlert.ResolvedBy = alert.ResolvedBy.GetLogin()
			}

			r, err := json.Marshal(simplifiedAlert)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal simplified alert: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

func ListSecretScanningAlerts(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool(
			"list_secret_scanning_alerts",
			mcp.WithDescription(t("TOOL_LIST_SECRET_SCANNING_ALERTS_DESCRIPTION", "List secret scanning alerts in a GitHub repository.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_SECRET_SCANNING_ALERTS_USER_TITLE", "List secret scanning alerts"),
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
			mcp.WithString("state",
				mcp.Description("Filter by state"),
				mcp.Enum("open", "resolved"),
			),
			mcp.WithString("secret_type",
				mcp.Description("A comma-separated list of secret types to return. All default secret patterns are returned. To return generic patterns, pass the token name(s) in the parameter."),
			),
			mcp.WithString("resolution",
				mcp.Description("Filter by resolution"),
				mcp.Enum("false_positive", "wont_fix", "revoked", "pattern_edited", "pattern_deleted", "used_in_tests"),
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
			state, err := OptionalParam[string](request, "state")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			secretType, err := OptionalParam[string](request, "secret_type")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			resolution, err := OptionalParam[string](request, "resolution")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			alerts, resp, err := client.SecretScanning.ListAlertsForRepo(ctx, owner, repo, &github.SecretScanningAlertListOptions{State: state, SecretType: secretType, Resolution: resolution})
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

			// Create simplified alert structure
			type SimplifiedSecretScanningAlert struct {
				Number                int64  `json:"number"`
				CreatedAt             string `json:"created_at,omitempty"`
				UpdatedAt             string `json:"updated_at,omitempty"`
				HTMLURL               string `json:"html_url,omitempty"`
				State                 string `json:"state,omitempty"`
				Resolution            string `json:"resolution,omitempty"`
				ResolvedAt            string `json:"resolved_at,omitempty"`
				ResolvedBy            string `json:"resolved_by,omitempty"`
				SecretType            string `json:"secret_type,omitempty"`
				SecretTypeDisplayName string `json:"secret_type_display_name,omitempty"`
				Secret                string `json:"secret,omitempty"`
			}

			// Create list of simplified alerts
			simplifiedAlerts := []SimplifiedSecretScanningAlert{}
			for _, alert := range alerts {
				// Create simplified alert instance
				simplifiedAlert := SimplifiedSecretScanningAlert{
					Number:                int64(alert.GetNumber()),
					HTMLURL:               alert.GetHTMLURL(),
					State:                 alert.GetState(),
					Resolution:            alert.GetResolution(),
					SecretType:            alert.GetSecretType(),
					SecretTypeDisplayName: alert.GetSecretTypeDisplayName(),
					Secret:                alert.GetSecret(),
				}

				// Format dates
				if alert.CreatedAt != nil {
					simplifiedAlert.CreatedAt = alert.CreatedAt.Format(time.RFC3339)
				}
				if alert.UpdatedAt != nil {
					simplifiedAlert.UpdatedAt = alert.UpdatedAt.Format(time.RFC3339)
				}
				if alert.ResolvedAt != nil {
					simplifiedAlert.ResolvedAt = alert.ResolvedAt.Format(time.RFC3339)
				}

				// Add user information
				if alert.ResolvedBy != nil {
					simplifiedAlert.ResolvedBy = alert.ResolvedBy.GetLogin()
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
