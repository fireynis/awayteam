package server

import (
	"io/fs"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/jeremy/ai-dashboard/internal/config"
	"github.com/jeremy/ai-dashboard/internal/store"
	"github.com/jeremy/ai-dashboard/internal/ws"
)

type Server struct {
	store      store.Store
	hub        *ws.Hub
	config     config.Config
	upgrader   websocket.Upgrader
	frontendFS fs.FS
}

func New(cfg config.Config, st store.Store, h *ws.Hub) *Server {
	return &Server{
		store:  st,
		hub:    h,
		config: cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (s *Server) SetFrontendFS(fsys fs.FS) {
	s.frontendFS = fsys
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("POST /api/v1/events", s.handlePostEvent)
	mux.HandleFunc("GET /api/v1/agents", s.handleGetAgents)
	mux.HandleFunc("GET /api/v1/agents/{id}/events", s.handleGetAgentEvents)
	mux.HandleFunc("GET /api/v1/ws", s.handleWS)
	mux.HandleFunc("GET /api/v1/ws/agents/{id}", s.handleAgentWS)

	if s.frontendFS != nil {
		mux.Handle("/", http.FileServerFS(s.frontendFS))
	}

	return corsMiddleware(mux)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	ch := make(chan []byte, 256)
	s.hub.Register(ch)

	go func() {
		defer conn.Close()
		for msg := range ch {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				break
			}
		}
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	s.hub.Unregister(ch)
}

func (s *Server) handleAgentWS(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	ch := make(chan []byte, 256)
	s.hub.Register(ch)

	go func() {
		defer conn.Close()
		for msg := range ch {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				break
			}
		}
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		s.hub.RouteResponse(agentID, msg)
	}

	s.hub.Unregister(ch)
}
