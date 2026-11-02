package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/bytedance/crypto-backtest/internal/backtest"
	"github.com/bytedance/crypto-backtest/internal/indicator"
	"github.com/bytedance/crypto-backtest/internal/strategy"
)

type Server struct {
	repository strategy.Repository
	registry   *indicator.Registry
	dataRoot   string
	runner     *backtest.Service
}

func NewServer(repository strategy.Repository, registry *indicator.Registry, dataRoot string) *Server {
	return &Server{
		repository: repository,
		registry:   registry,
		dataRoot:   dataRoot,
		runner:     backtest.NewService(repository, registry, dataRoot),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/v1/catalog", s.handleCatalog)
	mux.HandleFunc("GET /api/v1/strategies", s.handleListStrategies)
	mux.HandleFunc("POST /api/v1/strategies", s.handleCreateStrategy)
	mux.HandleFunc("GET /api/v1/strategies/{id}", s.handleGetStrategy)
	mux.HandleFunc("POST /api/v1/backtests", s.handleRunBacktest)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"indicators": s.registry.Definitions(),
	})
}

func (s *Server) handleListStrategies(w http.ResponseWriter, r *http.Request) {
	ids, err := s.repository.ListIDs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	summaries := make([]strategy.Summary, 0, len(ids))
	for _, id := range ids {
		definition, err := s.repository.LoadByID(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		summaries = append(summaries, strategy.Summarize(definition))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"strategies": summaries})
}

func (s *Server) handleCreateStrategy(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	definition, err := strategy.Decode(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := strategy.Validate(definition, s.registry); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.repository.Save(r.Context(), definition, raw); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"strategy": strategy.Summarize(definition),
	})
}

func (s *Server) handleGetStrategy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	definition, err := s.repository.LoadByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, definition)
}

func (s *Server) handleRunBacktest(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var request backtest.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(request.StrategyID) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("strategy_id is required"))
		return
	}
	response, err := s.runner.Run(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
