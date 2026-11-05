-- Apply idempotently alongside strategy_revisions before accepting runs.
-- State and artifact publication commit together; a result is visible only
-- when status = 'completed'. Snapshots are never overwritten.
CREATE TABLE IF NOT EXISTS backtest_runs (
  id TEXT PRIMARY KEY,
  strategy_id TEXT NOT NULL,
  strategy_revision INTEGER NOT NULL CHECK (strategy_revision >= 1),
  status TEXT NOT NULL CHECK (status IN ('queued','running','completed','failed','cancelled','interrupted')),
  progress INTEGER NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
  request_json TEXT NOT NULL,
  provenance_json TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  result_sha256 TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS backtest_runs_strategy_created
  ON backtest_runs(strategy_id, created_at DESC);
CREATE TABLE IF NOT EXISTS backtest_run_artifacts (
  run_id TEXT NOT NULL REFERENCES backtest_runs(id),
  kind TEXT NOT NULL CHECK (kind IN ('input','result')),
  sha256 TEXT NOT NULL,
  payload BLOB NOT NULL,
  PRIMARY KEY (run_id, kind)
);
CREATE TABLE IF NOT EXISTS backtest_run_events (
  run_id TEXT NOT NULL REFERENCES backtest_runs(id),
  sequence INTEGER NOT NULL,
  status TEXT NOT NULL,
  progress INTEGER NOT NULL,
  message TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (run_id, sequence)
);
