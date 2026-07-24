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
	"time"

	agenttools "github.com/Yacobolo/leapview/internal/agent/tools"
	"github.com/Yacobolo/leapview/internal/api"
	"github.com/spf13/cobra"
)

func agentCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "agent", Short: "Use the LeapView read-only agent"}
	ask := &cobra.Command{
		Use:   "ask [question]",
		Short: "Ask the LeapView read-only agent a question",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentAsk(ctx, opts, args[0])
		},
	}
	ask.Flags().StringVar(&opts.target, "target", "", "LeapView server URL")
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
	conversations.Flags().StringVar(&opts.target, "target", "", "LeapView server URL")
	conversations.Flags().StringVar(&opts.token, "token", "", "API token")
	conversations.Flags().BoolVar(&opts.jsonOutput, "json", false, "print JSON response")
	addPaginationFlags(conversations, opts)

	tools := &cobra.Command{
		Use:   "tools",
		Short: "List the canonical agent tools",
		Long:  "List the canonical agent tools exposed by built-in chat and deployment MCP, including each tool's privilege, effect, defaults, closed input schema, and backing operation.",
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
		if err := doJSONWithHeaders(ctx, http.MethodPost, agentConversationEndpoint(target, nil), token, map[string]string{"Idempotency-Key": fmt.Sprintf("cli-conversation-%d", time.Now().UnixNano())}, bytes.NewReader(body), &conversation); err != nil {
			return err
		}
		conversationID = conversation.ID
	}
	body, _ := json.Marshal(api.AgentTurnRequest{Input: question})
	var run api.AgentRunResponse
	if err := doJSONWithHeaders(ctx, http.MethodPost, agentRunEndpoint(target, conversationID, ""), token, map[string]string{"Idempotency-Key": fmt.Sprintf("cli-run-%d", time.Now().UnixNano())}, bytes.NewReader(body), &run); err != nil {
		return err
	}
	for run.Status == "queued" || run.Status == "running" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
		if err := doJSON(ctx, http.MethodGet, agentRunEndpoint(target, conversationID, run.ID), token, nil, &run); err != nil {
			return err
		}
	}
	var messages apiListResponse[api.AgentMessageResponse]
	if err := doJSON(ctx, http.MethodGet, agentMessagesEndpoint(target, conversationID), token, nil, &messages); err != nil {
		return err
	}
	content := ""
	for _, message := range messages.Items {
		if message.RunID == run.ID && message.Role == "assistant" && message.ContentText != "" {
			content = message.ContentText
		}
	}
	if opts.jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"conversationId": conversationID, "run": run, "content": content})
	}
	fmt.Println(content)
	fmt.Printf("\nconversation=%s run=%s stop=%s\n", conversationID, run.ID, run.StopReason)
	if run.Status != "completed" {
		return fmt.Errorf("agent run ended with status %s: %s", run.Status, run.Error)
	}
	return nil
}

func runAgentConversations(ctx context.Context, opts *rootOptions) error {
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return err
	}
	var response apiListResponse[api.AgentConversationResponse]
	if err := doJSON(ctx, http.MethodGet, agentConversationEndpoint(target, paginationQuery(opts)), token, nil, &response); err != nil {
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
	reference, err := agenttools.ReferenceCatalog()
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tPRIVILEGE\tEFFECT\tDEFAULTS\tINPUT_SCHEMA\tOPERATION")
	for _, tool := range reference {
		defaults, _ := json.Marshal(tool.Defaults)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			tool.Name, tool.Privilege, tool.Effect, defaults, cliCompactJSON(tool.InputSchema), tool.OperationID)
	}
	return tw.Flush()
}

func cliCompactJSON(value json.RawMessage) string {
	var output bytes.Buffer
	if err := json.Compact(&output, value); err != nil {
		return string(value)
	}
	return output.String()
}

func agentConversationEndpoint(target string, query url.Values) string {
	u, _ := apiOperationURL(target, "listAgentConversations", nil, query)
	return u
}

func agentRunEndpoint(target, conversationID, runID string) string {
	operation := "createAgentRun"
	params := map[string]string{"conversation": conversationID}
	if runID != "" {
		operation = "getAgentRun"
		params["run"] = runID
	}
	u, _ := apiOperationURL(target, operation, params, nil)
	return u
}

func agentMessagesEndpoint(target, conversationID string) string {
	u, _ := apiOperationURL(target, "listAgentMessages", map[string]string{"conversation": conversationID}, nil)
	return u
}
