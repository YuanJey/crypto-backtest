package backtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/YuanJey/crypto-backtest/internal/compiler"
	"github.com/YuanJey/crypto-backtest/internal/runstore"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

// Manager owns run lifetime; HTTP request lifetime only covers preparation
// and admission for asynchronous runs.
type Manager struct {
	service *Service
	store   runstore.Store
	mu      sync.Mutex
	active  map[string]context.CancelFunc
	closed  bool
	wg      sync.WaitGroup
	slots   chan struct{}
}

type RunInput struct {
	Definition strategy.Definition       `json:"definition"`
	Version    strategy.RevisionMetadata `json:"strategy_version"`
	Config     strategy.RunConfig        `json:"run_config"`
	Request    Request                   `json:"request"`
	Data       DataSnapshot              `json:"data"`
}

func NewManager(ctx context.Context, service *Service, store runstore.Store) (*Manager, error) {
	if err := store.Recover(ctx); err != nil {
		return nil, err
	}
	return &Manager{service: service, store: store, active: map[string]context.CancelFunc{}, slots: make(chan struct{}, 2)}, nil
}

func (m *Manager) Store() runstore.Store { return m.store }

func (m *Manager) admit(ctx context.Context, prepared PreparedRun) (string, context.Context, error) {
	input, err := json.Marshal(RunInput{Definition: prepared.Definition, Version: prepared.Version,
		Config: prepared.Config, Request: prepared.Request, Data: prepared.Data})
	if err != nil {
		return "", nil, err
	}
	request, err := json.Marshal(prepared.Request)
	if err != nil {
		return "", nil, err
	}
	config, err := json.Marshal(prepared.Config)
	if err != nil {
		return "", nil, err
	}
	plan, err := json.Marshal(prepared.Plan.Snapshot())
	if err != nil {
		return "", nil, err
	}
	provenance, err := json.Marshal(struct {
		StrategyVersion strategy.RevisionMetadata `json:"strategy_version"`
		Data            DataProvenance            `json:"data"`
		ConfigSHA256    string                    `json:"config_sha256"`
		PlanSHA256      string                    `json:"plan_sha256"`
		EngineVersion   string                    `json:"engine_version"`
	}{prepared.Version, prepared.Data.Provenance, runstore.Hash(config), runstore.Hash(plan), "bar-engine-v1.2-mmr-coverage"})
	if err != nil {
		return "", nil, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", nil, err
	}
	id := "run-" + hex.EncodeToString(random[:])
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return "", nil, fmt.Errorf("backtest manager is closed")
	}
	if len(m.active) >= 16 {
		return "", nil, fmt.Errorf("backtest queue is full")
	}
	if err := m.store.Create(ctx, runstore.Record{ID: id, StrategyID: prepared.Definition.ID,
		StrategyRevision: prepared.Version.Revision, Request: request, Provenance: provenance}, input); err != nil {
		return "", nil, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	m.active[id] = cancel
	m.wg.Add(1)
	return id, runCtx, nil
}

func (m *Manager) Start(ctx context.Context, request Request) (runstore.Record, error) {
	prepared, err := m.service.Prepare(ctx, request)
	if err != nil {
		return runstore.Record{}, err
	}
	return m.startPrepared(ctx, prepared)
}

func (m *Manager) startPrepared(ctx context.Context, prepared PreparedRun) (runstore.Record, error) {
	id, runCtx, err := m.admit(ctx, prepared)
	if err != nil {
		return runstore.Record{}, err
	}
	go func() { _, _ = m.execute(runCtx, id, prepared) }()
	return m.store.Get(ctx, id)
}

func (m *Manager) Run(ctx context.Context, request Request) (Response, error) {
	prepared, err := m.service.Prepare(ctx, request)
	if err != nil {
		return Response{}, err
	}
	id, runCtx, err := m.admit(ctx, prepared)
	if err != nil {
		return Response{}, err
	}
	stop := context.AfterFunc(ctx, func() { _, _ = m.Cancel(context.Background(), id) })
	defer stop()
	return m.execute(runCtx, id, prepared)
}

func (m *Manager) execute(ctx context.Context, id string, prepared PreparedRun) (response Response, err error) {
	defer func() {
		m.mu.Lock()
		if cancel := m.active[id]; cancel != nil {
			cancel()
		}
		delete(m.active, id)
		m.mu.Unlock()
		m.wg.Done()
	}()
	select {
	case m.slots <- struct{}{}:
		defer func() { <-m.slots }()
	case <-ctx.Done():
		_ = m.store.Transition(context.Background(), id, "cancelled", 0, "", nil)
		return Response{}, ctx.Err()
	}
	if err := m.store.Transition(context.Background(), id, "running", 0, "", nil); err != nil {
		return Response{}, err
	}
	previousProgress := 0
	response, err = m.service.Execute(ctx, prepared, func(completed, total int) {
		progress := completed * 99 / total
		if progress > previousProgress {
			_ = m.store.Transition(context.Background(), id, "running", progress, "", nil)
			previousProgress = progress
		}
	})
	if err != nil {
		status, message := "failed", err.Error()
		if errors.Is(err, context.Canceled) {
			status, message = "cancelled", ""
		}
		_ = m.store.Transition(context.Background(), id, status, previousProgress, message, nil)
		return Response{}, err
	}
	response.RunID = id
	raw, err := json.Marshal(response)
	if err != nil {
		_ = m.store.Transition(context.Background(), id, "failed", previousProgress, err.Error(), nil)
		return Response{}, err
	}
	if err := m.store.Transition(context.Background(), id, "completed", 100, "", raw); err != nil {
		return Response{}, err
	}
	return response, nil
}

func (m *Manager) Cancel(ctx context.Context, id string) (runstore.Record, error) {
	err := m.store.Transition(ctx, id, "cancelled", 0, "", nil)
	if err != nil && !errors.Is(err, runstore.ErrTerminal) {
		return runstore.Record{}, err
	}
	m.mu.Lock()
	if cancel := m.active[id]; cancel != nil {
		cancel()
	}
	m.mu.Unlock()
	return m.store.Get(ctx, id)
}

func (m *Manager) Result(ctx context.Context, id string) (json.RawMessage, error) {
	raw, err := m.store.Artifact(ctx, id, "result")
	return json.RawMessage(raw), err
}

// Replay recompiles the stored logical snapshot and consumes stored bars.
// Changes to a live strategy or dataset cannot affect this experiment.
func (m *Manager) Replay(ctx context.Context, id string) (runstore.Record, error) {
	raw, err := m.store.Artifact(ctx, id, "input")
	if err != nil {
		return runstore.Record{}, err
	}
	var input RunInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return runstore.Record{}, err
	}
	plan, err := compiler.Compile(input.Definition, m.service.registry)
	if err != nil {
		return runstore.Record{}, err
	}
	return m.startPrepared(ctx, PreparedRun{Request: input.Request, Definition: input.Definition,
		Version: input.Version, Config: input.Config, Data: input.Data, Plan: plan})
}

func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	for _, cancel := range m.active {
		cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}
