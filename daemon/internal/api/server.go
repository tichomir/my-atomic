package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/tichomir/my-atomic/daemon/internal/agent"
	"github.com/tichomir/my-atomic/daemon/internal/audit"
	"github.com/tichomir/my-atomic/daemon/internal/policy"
)

// Server exposes the atomicagentd API over a Unix domain socket.
// The socket is owned by atomic-admin group (mode 0660) so that
// members of that group can use atomic-agent-ctl without sudo.
type Server struct {
	socketPath      string
	listener        net.Listener
	httpServer      *http.Server
	falcoHTTPServer *http.Server
	manager         *agent.Manager
	policyEng       *policy.Engine
	auditor         *audit.Logger
	logger          *slog.Logger
}

// NewServer creates an API server.
func NewServer(socketPath string, mgr *agent.Manager, eng *policy.Engine, aud *audit.Logger) *Server {
	s := &Server{
		socketPath: socketPath,
		manager:    mgr,
		policyEng:  eng,
		auditor:    aud,
		logger:     slog.Default(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/agents", s.handleRegisterAgent)
	mux.HandleFunc("GET /v1/agents", s.handleListAgents)
	mux.HandleFunc("GET /v1/agents/{id}", s.handleGetAgent)
	mux.HandleFunc("POST /v1/agents/{id}/start", s.handleStartAgent)
	mux.HandleFunc("POST /v1/agents/{id}/stop", s.handleStopAgent)
	mux.HandleFunc("POST /v1/agents/{id}/unregister", s.handleUnregisterAgent)
	mux.HandleFunc("DELETE /v1/agents/{id}", s.handleKillAgent)
	mux.HandleFunc("POST /v1/policy/evaluate", s.handleEvaluatePolicy)
	mux.HandleFunc("POST /v1/policy/reload", s.handleReloadPolicy)
	mux.HandleFunc("GET /v1/health", s.handleHealth)

	// Falco webhook receiver
	mux.HandleFunc("POST /falco", s.handleFalcoWebhook)

	s.httpServer = &http.Server{Handler: mux}
	return s
}

// Start begins listening on the Unix socket.
func (s *Server) Start() error {
	// Remove stale socket file
	os.Remove(s.socketPath)

	l, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.socketPath, err)
	}

	// Set socket permissions: group-readable for atomic-admin members
	if err := os.Chmod(s.socketPath, 0660); err != nil {
		l.Close()
		return fmt.Errorf("setting socket permissions: %w", err)
	}

	s.listener = l
	s.logger.Info("API server listening", "socket", s.socketPath)

	go func() {
		if err := s.httpServer.Serve(l); err != nil && err != http.ErrServerClosed {
			s.logger.Error("API server error", "error", err)
		}
	}()
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.falcoHTTPServer != nil {
		_ = s.falcoHTTPServer.Shutdown(ctx)
	}
	return s.httpServer.Shutdown(ctx)
}

// StartFalcoWebhookListener binds a TCP HTTP server so Falco's http_output can
// POST events to atomicagentd. Falco cannot post to a Unix socket, so we need
// this separate listener. addr is typically "127.0.0.1:9765".
func (s *Server) StartFalcoWebhookListener(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /falco", s.handleFalcoWebhook)

	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("binding Falco webhook listener on %s: %w", addr, err)
	}

	s.falcoHTTPServer = &http.Server{Handler: mux}
	go func() {
		if err := s.falcoHTTPServer.Serve(l); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Falco webhook listener error", "error", err)
		}
	}()
	s.logger.Info("Falco webhook listener started", "addr", addr)
	return nil
}

// --- Request/Response types ---

type RegisterAgentRequest struct {
	ID      string `json:"id"`
	Profile string `json:"profile"`
	Exec    string `json:"exec"`
}

type AgentResponse struct {
	ID        string `json:"id"`
	Profile   string `json:"profile"`
	State     string `json:"state"`
	Workspace string `json:"workspace"`
}

type PolicyEvalRequest struct {
	AgentID    string            `json:"agent_id"`
	ActionType string            `json:"action_type"`
	Resource   string            `json:"resource"`
	Context    map[string]string `json:"context,omitempty"`
}

type FalcoWebhookPayload struct {
	Output   string `json:"output"`
	Priority string `json:"priority"`
	Rule     string `json:"rule"`
	Time     string `json:"time"`
	Fields   struct {
		ContainerID   string `json:"container.id"`
		ProcEnvAtomic string `json:"proc.env[ATOMIC_AGENT_ID]"`
	} `json:"output_fields"`
}

// --- Handlers ---

func (s *Server) handleRegisterAgent(w http.ResponseWriter, r *http.Request) {
	var req RegisterAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Profile == "" {
		req.Profile = "minimal"
	}

	ra, err := s.manager.Register(req.ID, req.Profile, req.Exec)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, AgentResponse{
		ID:        ra.ID,
		Profile:   ra.Profile,
		State:     string(ra.State),
		Workspace: ra.WorkspacePath,
	})
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents := s.manager.ListAgents()
	result := make([]AgentResponse, 0, len(agents))
	for _, a := range agents {
		result = append(result, AgentResponse{
			ID:        a.ID,
			Profile:   a.Profile,
			State:     string(a.State),
			Workspace: a.WorkspacePath,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ra, err := s.manager.GetStatus(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, AgentResponse{
		ID:        ra.ID,
		Profile:   ra.Profile,
		State:     string(ra.State),
		Workspace: ra.WorkspacePath,
	})
}

func (s *Server) handleStartAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.manager.Start(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (s *Server) handleStopAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.manager.Stop(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) handleKillAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	reason := "operator requested kill"
	if err := s.manager.Kill(id, reason); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "killed"})
}

func (s *Server) handleUnregisterAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	purge := r.URL.Query().Get("purge") == "true"
	if err := s.manager.Unregister(id, purge); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unregistered"})
}

func (s *Server) handleEvaluatePolicy(w http.ResponseWriter, r *http.Request) {
	var req PolicyEvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	action := policy.AgentAction{
		AgentID:    req.AgentID,
		ActionType: req.ActionType,
		Resource:   req.Resource,
		Context:    req.Context,
	}

	decision, err := s.policyEng.Evaluate(r.Context(), action)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.auditor.LogAgentAction(
		req.AgentID, req.ActionType, req.Resource,
		decision.Profile,
		map[bool]string{true: "allow", false: "deny"}[decision.Allow],
		decision.Reasons,
	)

	writeJSON(w, http.StatusOK, decision)
}

func (s *Server) handleReloadPolicy(w http.ResponseWriter, r *http.Request) {
	if err := s.policyEng.Reload(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	e := audit.NewEvent(audit.EventPolicyReloaded, audit.SeverityInfo, "policies reloaded by operator")
	s.auditor.Log(e)
	writeJSON(w, http.StatusOK, map[string]string{"status": "reloaded"})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleFalcoWebhook receives runtime alerts from Falco and takes automated action.
func (s *Server) handleFalcoWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "reading body")
		return
	}

	var payload FalcoWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		s.logger.Warn("malformed Falco webhook payload", "error", err)
		w.WriteHeader(http.StatusOK) // Don't break Falco on parse errors
		return
	}

	// Normalize: Falco may render hyphens as underscores in output text; agent IDs use hyphens.
	agentID := strings.ReplaceAll(payload.Fields.ProcEnvAtomic, "_", "-")
	isCritical := strings.ToUpper(payload.Priority) == "CRITICAL"

	s.auditor.LogRuntimeAlert(agentID, payload.Rule, payload.Output, isCritical)

	// Auto-kill agent on CRITICAL alerts if it's a known agent
	if isCritical && agentID != "" {
		s.logger.Warn("CRITICAL Falco alert - killing agent",
			"agent_id", agentID,
			"rule", payload.Rule,
		)
		if err := s.manager.Kill(agentID, fmt.Sprintf("falco critical: %s", payload.Rule)); err != nil {
			s.logger.Error("failed to kill agent after critical alert",
				"agent_id", agentID,
				"error", err,
			)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
