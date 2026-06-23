package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"text/tabwriter"

	"github.com/Yacobolo/libredash/internal/api"
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

	parent.AddCommand(ask, conversations)
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
		if err := doJSON(ctx, http.MethodPost, agentConversationEndpoint(target, opts.workspaceID), token, bytes.NewReader(body), &conversation); err != nil {
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
	var rows []api.AgentConversationResponse
	if err := doJSON(ctx, http.MethodGet, agentConversationEndpoint(target, opts.workspaceID), token, nil, &rows); err != nil {
		return err
	}
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

func agentConversationEndpoint(target, workspaceID string) string {
	u, _ := url.Parse(target + "/api/workspaces/" + url.PathEscape(workspaceID) + "/agent/conversations")
	return u.String()
}

func agentTurnEndpoint(target, workspaceID, conversationID string) string {
	u, _ := url.Parse(target + "/api/workspaces/" + url.PathEscape(workspaceID) + "/agent/conversations/" + url.PathEscape(conversationID) + "/turns")
	return u.String()
}
