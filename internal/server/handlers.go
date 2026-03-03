package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/jeremy/awayteam/internal/events"
	"github.com/jeremy/awayteam/internal/store"
)

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handlePostEvent(w http.ResponseWriter, r *http.Request) {
	var evt events.Event
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := evt.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.SaveEvent(r.Context(), evt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save event")
		return
	}

	data, err := json.Marshal(evt)
	if err == nil {
		s.hub.Broadcast(data)
	}

	writeJSON(w, http.StatusCreated, evt)
}

func (s *Server) handleGetAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.store.GetAgents(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}
	if agents == nil {
		agents = []store.AgentState{}
	}
	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) handleGetAgentEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "agent id is required")
		return
	}

	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	evts, err := s.store.GetAgentEvents(r.Context(), id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get events")
		return
	}
	if evts == nil {
		evts = []events.Event{}
	}
	writeJSON(w, http.StatusOK, evts)
}

func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}

	sessionData, err := s.store.GetAgentSessionData(r.Context(), agentID)
	if err != nil {
		http.Error(w, "failed to look up agent", http.StatusInternalServerError)
		return
	}

	var tmuxSession string
	if sessionData != nil {
		var info struct {
			TmuxSession string `json:"tmux_session"`
		}
		if json.Unmarshal(sessionData, &info) == nil {
			tmuxSession = info.TmuxSession
		}
	}

	s.terminal.ServeWebSocket(w, r, tmuxSession)
}

func (s *Server) handleShellWS(w http.ResponseWriter, r *http.Request) {
	s.terminal.ServeWebSocket(w, r, "")
}
