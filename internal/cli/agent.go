package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/Yacobolo/libredash/internal/api"
	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	"github.com/spf13/cobra"
)

func agentCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "agent", Short: "Use the LibreDash read-only agent"}
	ask := &cobra.Command{
		Use:   "ask [question]",
		Short: "Ask the LibreDash read-only agent a question",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentAsk(ctx, opts, args[0])
		},
	}
	ask.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	ask.Flags().StringVar(&opts.token, "token", "", "API token")
	ask.Flags().StringVar(&opts.conversation, "conversation", "", "existing agent conversation id")
	ask.Flags().BoolVar(&opts.jsonOutput, "json", false, "print JSON response")

	conversations := &cobra.Command{
		Use:   "conversations",
		Short: "List agent conversations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentConversations(ctx, opts)
		},
	}
	conversations.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	conversations.Flags().StringVar(&opts.token, "token", "", "API token")
	conversations.Flags().BoolVar(&opts.jsonOutput, "json", false, "print JSON response")
	addPaginationFlags(conversations, opts)

	tools := &cobra.Command{
		Use:   "tools",
		Short: "List generated agent tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentTools()
		},
	}

	parent.AddCommand(ask, conversations, tools)
	return parent
}

func runAgentAsk(ctx context.Context, opts *rootOptions, question string) error {
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return err
	}
	conversationID := opts.conversation
	if conversationID == "" {
		body, _ := json.Marshal(api.AgentConversationCreateRequest{Title: "CLI conversation"})
		var conversation api.AgentConversationResponse
		if err := doJSON(ctx, http.MethodPost, agentConversationEndpoint(target, opts.workspaceID, nil), token, bytes.NewReader(body), &conversation); err != nil {
			return err
		}
		conversationID = conversation.ID
	}
	body, _ := json.Marshal(api.AgentTurnRequest{Input: question})
	var result api.AgentTurnResponse
	if err := doJSON(ctx, http.MethodPost, agentTurnEndpoint(target, opts.workspaceID, conversationID), token, bytes.NewReader(body), &result); err != nil {
		return err
	}
	if opts.jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	fmt.Println(result.Content)
	fmt.Printf("\nconversation=%s run=%s stop=%s\n", result.ConversationID, result.RunID, result.StopReason)
	return nil
}

func runAgentConversations(ctx context.Context, opts *rootOptions) error {
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return err
	}
	var response apiListResponse[api.AgentConversationResponse]
	if err := doJSON(ctx, http.MethodGet, agentConversationEndpoint(target, opts.workspaceID, paginationQuery(opts)), token, nil, &response); err != nil {
		return err
	}
	rows := response.Items
	if opts.jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(rows)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tTITLE\tUPDATED")
	for _, row := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", row.ID, row.Status, row.Title, row.UpdatedAt)
	}
	return tw.Flush()
}

func runAgentTools() error {
	type row struct {
		name         string
		operationID  string
		privilege    string
		risk         string
		defaultLimit int
	}
	contracts := apigenapi.GetAPIGenOperationContracts()
	rows := make([]row, 0, len(contracts))
	for _, contract := range contracts {
		agentExtension, ok := contract.Extensions["x-agent"].(map[string]any)
		if !ok || !cliBoolFromMap(agentExtension, "enabled") {
			continue
		}
		name := cliStringFromMap(agentExtension, "name")
		risk := cliStringFromMap(agentExtension, "risk")
		authz, _ := contract.Extensions["x-authz"].(map[string]any)
		rows = append(rows, row{
			name:         name,
			operationID:  contract.OperationID,
			privilege:    cliStringFromMap(authz, "privilege"),
			risk:         risk,
			defaultLimit: cliIntFromMap(agentExtension, "defaultLimit"),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].name < rows[j].name
	})
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tPRIVILEGE\tRISK\tDEFAULT_LIMIT\tOPERATION")
	for _, row := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", row.name, row.privilege, row.risk, row.defaultLimit, row.operationID)
	}
	return tw.Flush()
}

func agentConversationEndpoint(target, workspaceID string, query url.Values) string {
	u, _ := apiOperationURL(target, "listAgentConversations", map[string]string{"workspace": workspaceID}, query)
	return u
}

func agentTurnEndpoint(target, workspaceID, conversationID string) string {
	u, _ := apiOperationURL(target, "createAgentTurn", map[string]string{"workspace": workspaceID, "conversation": conversationID}, nil)
	return u
}

func cliStringFromMap(values map[string]any, key string) string {
	if value, ok := values[key].(string); ok {
		return value
	}
	return ""
}

func cliBoolFromMap(values map[string]any, key string) bool {
	if value, ok := values[key].(bool); ok {
		return value
	}
	return false
}

func cliIntFromMap(values map[string]any, key string) int {
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, _ := value.Int64()
		return int(parsed)
	default:
		return 0
	}
}
