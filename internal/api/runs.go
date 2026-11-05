package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/YuanJey/crypto-backtest/internal/backtest"
	"github.com/YuanJey/crypto-backtest/internal/runstore"
)

func runError(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	if errors.Is(err, runstore.ErrNotFound) {
		code = http.StatusNotFound
	}
	if errors.Is(err, runstore.ErrNotCompleted) || errors.Is(err, runstore.ErrTerminal) {
		code = http.StatusConflict
	}
	writeError(w, code, err)
}

func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var request backtest.Request
	if err := decodeStrictJSON(r.Body, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	record, err := s.runs.Start(r.Context(), request)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, http.StatusAccepted, record)
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	records, err := s.runs.Store().List(r.Context(), r.URL.Query().Get("strategy_id"), 50)
	if err != nil {
		runError(w, err)
		return
	}
	writeJSON(w, 200, map[string]interface{}{"runs": records})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	record, err := s.runs.Store().Get(r.Context(), r.PathValue("id"))
	if err != nil {
		runError(w, err)
		return
	}
	writeJSON(w, 200, record)
}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	record, err := s.runs.Cancel(r.Context(), r.PathValue("id"))
	if err != nil {
		runError(w, err)
		return
	}
	writeJSON(w, 200, record)
}

func (s *Server) handleRunResult(w http.ResponseWriter, r *http.Request) {
	result, err := s.runs.Result(r.Context(), r.PathValue("id"))
	if err != nil {
		runError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.runs.Store().Events(r.Context(), r.PathValue("id"))
	if err != nil {
		runError(w, err)
		return
	}
	writeJSON(w, 200, map[string]interface{}{"events": events})
}

func (s *Server) handleRunSnapshot(w http.ResponseWriter, r *http.Request) {
	raw, err := s.runs.Store().Artifact(r.Context(), r.PathValue("id"), "input")
	if err != nil {
		runError(w, err)
		return
	}
	// Return precisely the hash-verified input used by the archived run.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (s *Server) handleReplayRun(w http.ResponseWriter, r *http.Request) {
	record, err := s.runs.Replay(r.Context(), r.PathValue("id"))
	if err != nil {
		runError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, record)
}

func (s *Server) handleStrategySnapshot(w http.ResponseWriter, r *http.Request) {
	revision := 0
	if value := strings.TrimSpace(r.URL.Query().Get("revision")); value != "" {
		var err error
		revision, err = strconv.Atoi(value)
		if err != nil || revision < 1 {
			writeError(w, 400, fmt.Errorf("revision must be a positive integer"))
			return
		}
	}
	definition, plan, version, err := s.runner.CompileStrategyRevision(r.Context(), r.PathValue("id"), revision)
	if err != nil {
		writeError(w, 404, err)
		return
	}
	snapshot := plan.Snapshot()
	writeJSON(w, 200, map[string]interface{}{"definition": definition, "strategy_version": version,
		"logic": snapshot.Logic, "default_run_config": snapshot.Defaults})
}
