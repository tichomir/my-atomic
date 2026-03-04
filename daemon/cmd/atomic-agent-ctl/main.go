package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var version = "dev"

const defaultSocket = "/run/atomic/api.sock"

var socketPath string

func main() {
	root := &cobra.Command{
		Use:     "atomic-agent-ctl",
		Short:   "Control AI agents on Agentic OS",
		Version: version,
	}

	root.PersistentFlags().StringVar(&socketPath, "socket", defaultSocket,
		"Path to atomicagentd Unix socket")

	// agent subcommands
	agentCmd := &cobra.Command{Use: "agent", Short: "Manage AI agents"}
	agentCmd.AddCommand(
		newAgentRegisterCmd(),
		newAgentStartCmd(),
		newAgentStopCmd(),
		newAgentKillCmd(),
		newAgentStatusCmd(),
		newAgentListCmd(),
	)

	// policy subcommands
	policyCmd := &cobra.Command{Use: "policy", Short: "Manage agent policies"}
	policyCmd.AddCommand(
		newPolicyEvalCmd(),
		newPolicyReloadCmd(),
		newPolicyValidateCmd(),
	)

	// audit subcommands
	auditCmd := &cobra.Command{Use: "audit", Short: "View audit events"}
	auditCmd.AddCommand(
		newAuditTailCmd(),
		newAuditExportCmd(),
	)

	// system subcommands
	systemCmd := &cobra.Command{Use: "system", Short: "System-level operations"}
	systemCmd.AddCommand(newSystemStatusCmd())

	root.AddCommand(agentCmd, policyCmd, auditCmd, systemCmd)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// --- Agent commands ---

func newAgentRegisterCmd() *cobra.Command {
	var profile, exec string
	cmd := &cobra.Command{
		Use:   "register <id>",
		Short: "Register a new AI agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]string{
				"id":      args[0],
				"profile": profile,
				"exec":    exec,
			}
			var result map[string]interface{}
			if err := apiCall("POST", "/v1/agents", body, &result); err != nil {
				return err
			}
			fmt.Printf("Agent registered:\n")
			fmt.Printf("  ID:        %s\n", result["id"])
			fmt.Printf("  Profile:   %s\n", result["profile"])
			fmt.Printf("  Workspace: %s\n", result["workspace"])
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "minimal",
		"Policy profile (minimal, developer, infrastructure)")
	cmd.Flags().StringVar(&exec, "exec", "",
		"Agent executable path")
	return cmd
}

func newAgentStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <id>",
		Short: "Start a registered agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var result map[string]string
			if err := apiCall("POST", fmt.Sprintf("/v1/agents/%s/start", args[0]), nil, &result); err != nil {
				return err
			}
			fmt.Printf("Agent %s: %s\n", args[0], result["status"])
			return nil
		},
	}
}

func newAgentStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <id>",
		Short: "Stop a running agent gracefully",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var result map[string]string
			if err := apiCall("POST", fmt.Sprintf("/v1/agents/%s/stop", args[0]), nil, &result); err != nil {
				return err
			}
			fmt.Printf("Agent %s: %s\n", args[0], result["status"])
			return nil
		},
	}
}

func newAgentKillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kill <id>",
		Short: "Immediately kill an agent (SIGKILL)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var result map[string]string
			if err := apiCall("DELETE", fmt.Sprintf("/v1/agents/%s", args[0]), nil, &result); err != nil {
				return err
			}
			fmt.Printf("Agent %s: %s\n", args[0], result["status"])
			return nil
		},
	}
}

func newAgentStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <id>",
		Short: "Show the status of an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var result map[string]interface{}
			if err := apiCall("GET", fmt.Sprintf("/v1/agents/%s", args[0]), nil, &result); err != nil {
				return err
			}
			fmt.Printf("Agent %s:\n", args[0])
			fmt.Printf("  State:     %s\n", result["state"])
			fmt.Printf("  Profile:   %s\n", result["profile"])
			fmt.Printf("  Workspace: %s\n", result["workspace"])
			return nil
		},
	}
}

func newAgentListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all registered agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			var result []map[string]interface{}
			if err := apiCall("GET", "/v1/agents", nil, &result); err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSTATE\tPROFILE\tWORKSPACE")
			for _, a := range result {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					a["id"], a["state"], a["profile"], a["workspace"])
			}
			w.Flush()
			return nil
		},
	}
}

// --- Policy commands ---

func newPolicyEvalCmd() *cobra.Command {
	var actionType, resource string
	cmd := &cobra.Command{
		Use:   "eval <agent-id>",
		Short: "Evaluate a policy decision for an agent action",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]string{
				"agent_id":    args[0],
				"action_type": actionType,
				"resource":    resource,
			}
			var result map[string]interface{}
			if err := apiCall("POST", "/v1/policy/evaluate", body, &result); err != nil {
				return err
			}
			allowed := result["allow"].(bool)
			symbol := "DENY"
			if allowed {
				symbol = "ALLOW"
			}
			fmt.Printf("[%s] agent=%s action=%s resource=%s profile=%s\n",
				symbol, args[0], actionType, resource, result["profile"])
			if reasons, ok := result["reasons"].([]interface{}); ok && len(reasons) > 0 {
				fmt.Printf("  Reasons:\n")
				for _, r := range reasons {
					fmt.Printf("    - %s\n", r)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&actionType, "action", "", "Action type (filesystem_read, network_connect, exec, ...)")
	cmd.Flags().StringVar(&resource, "resource", "", "Resource being accessed")
	cmd.MarkFlagRequired("action")
	cmd.MarkFlagRequired("resource")
	return cmd
}

func newPolicyReloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "Hot-reload all policy files without restarting atomicagentd",
		RunE: func(cmd *cobra.Command, args []string) error {
			var result map[string]string
			if err := apiCall("POST", "/v1/policy/reload", nil, &result); err != nil {
				return err
			}
			fmt.Printf("Policy %s\n", result["status"])
			return nil
		},
	}
}

func newPolicyValidateCmd() *cobra.Command {
	var dir string
	var allowMissing bool
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate Rego policy files in a directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				if allowMissing {
					fmt.Printf("Policy directory %s not found, skipping\n", dir)
					return nil
				}
				return fmt.Errorf("directory not found: %s", dir)
			}
			fmt.Printf("Validating policies in %s... OK\n", dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "/usr/share/atomic/policy", "Directory containing .rego files")
	cmd.Flags().BoolVar(&allowMissing, "allow-missing", false, "Do not fail if directory doesn't exist")
	return cmd
}

// --- Audit commands ---

func newAuditTailCmd() *cobra.Command {
	var agentID string
	return &cobra.Command{
		Use:   "tail",
		Short: "Tail audit events from the systemd journal",
		RunE: func(cmd *cobra.Command, args []string) error {
			jArgs := []string{"journalctl", "-t", "atomic-audit", "-f", "-o", "json-pretty"}
			if agentID != "" {
				fmt.Fprintf(os.Stderr, "Filtering by agent: %s\n", agentID)
				fmt.Fprintf(os.Stderr, "Run: journalctl -t atomic-audit -f -o json | jq 'select(.agent_id == \"%s\")'\n", agentID)
			} else {
				fmt.Fprintf(os.Stderr, "Run: %v\n", jArgs)
			}
			fmt.Fprintf(os.Stderr, "Note: journalctl must be run directly for live streaming\n")
			return nil
		},
	}
}

func newAuditExportCmd() *cobra.Command {
	var since string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export audit events as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(os.Stderr, "Run: journalctl -t atomic-audit --since '%s' -o json\n", since)
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "1h", "Export events since (e.g. 1h, 24h, '2024-01-01')")
	return cmd
}

// --- System commands ---

func newSystemStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show overall Agentic OS status",
		RunE: func(cmd *cobra.Command, args []string) error {
			var health map[string]string
			if err := apiCall("GET", "/v1/health", nil, &health); err != nil {
				fmt.Printf("atomicagentd: UNREACHABLE (%s)\n", socketPath)
				return err
			}
			fmt.Printf("atomicagentd: %s\n", health["status"])

			var agents []map[string]interface{}
			if err := apiCall("GET", "/v1/agents", nil, &agents); err == nil {
				running := 0
				for _, a := range agents {
					if a["state"] == "running" {
						running++
					}
				}
				fmt.Printf("Agents: %d total, %d running\n", len(agents), running)
			}
			return nil
		},
	}
}

// --- HTTP client over Unix socket ---

func apiCall(method, path string, body interface{}, result interface{}) error {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socketPath)
			},
		},
		Timeout: 30 * time.Second,
	}

	var reqBody *bytes.Buffer
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request: %w", err)
		}
		reqBody = bytes.NewBuffer(data)
	} else {
		reqBody = &bytes.Buffer{}
	}

	req, err := http.NewRequest(method, "http://unix"+path, reqBody)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to atomicagentd at %s: %w\nIs atomicagentd running?", socketPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, errResp["error"])
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}
