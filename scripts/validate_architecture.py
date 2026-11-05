"""Fixed real-market comparison against the recorded phase-one evidence.

Run against the isolated preview on port 8083. Existing completed archives are
reused; missing reference strategies are saved only in that preview database.
"""
import copy
import hashlib
import json
import math
from pathlib import Path
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
BASE = "http://127.0.0.1:8083/api/v1"


def call(path, body=None):
    request = urllib.request.Request(BASE + path, data=None if body is None else json.dumps(body).encode(),
                                     headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(request, timeout=60) as response:
        return json.load(response)


def sha(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()).hexdigest()


def audit_ledger(response):
    cash = response["run_config"]["account"]["initial_cash"]
    qty, avg = 0.0, 0.0
    seen = set()
    for index, event in enumerate(response["report"]["ledger"]):
        fill = event["fill"]
        assert event["sequence"] == index + 1 and fill["order_id"] not in seen
        seen.add(fill["order_id"])
        q, price, fee = fill["quantity"], fill["price"], fill["fee"]
        assert math.isclose(event["notional"], abs(q) * price, rel_tol=1e-10)
        if event["action"] == "close":
            realized = (price - avg) * q
            assert math.isclose(realized, fill["realized_pnl"], rel_tol=1e-10, abs_tol=1e-8)
            cash += (event["notional"] if response["run_config"]["account"]["market_type"] == "spot" else realized) - fee
            qty -= q
            if abs(qty) < 1e-12:
                qty, avg = 0.0, 0.0
        else:
            cash -= fee + (event["notional"] if response["run_config"]["account"]["market_type"] == "spot" else 0)
            new_qty = qty + q
            avg = (abs(qty) * avg + abs(q) * price) / abs(new_qty)
            qty = new_qty
        assert math.isclose(event["cash"], cash, abs_tol=1e-8, rel_tol=1e-10)
        assert math.isclose(event["position_qty"], qty, abs_tol=1e-10)
        assert math.isclose(event["average_price"], avg, abs_tol=1e-8, rel_tol=1e-10)
    last = response["report"]["equity_curve"][-1]
    equity = cash + (qty * last["close"] if response["run_config"]["account"]["market_type"] == "spot"
                     else qty * (last["close"] - avg))
    assert math.isclose(equity, response["final_equity"], abs_tol=1e-8, rel_tol=1e-10)
    return {"events": len(seen), "cash": cash, "position_qty": qty, "final_equity": equity, "passed": True}


baseline = json.loads((ROOT / "strategy-architecture-phase1-validation.json").read_text())
stored_runs = call("/backtest-runs")["runs"]
evidence = {"baseline_commit": baseline["baseline_commit"], "baseline_evidence": "strategy-architecture-phase1-validation.json",
            "preview_port": 8083, "sha256_encoding": baseline["sha256_encoding"], "cases": [], "history": []}
for case in baseline["cases"] + baseline["negative_offset_witnesses"]:
    existing = next((run for run in stored_runs if run["strategy_id"] == case["id"]
                     and run["status"] == "completed"
                     and run["request"]["run_config"]["account"]["initial_cash"] == 10000), None)
    if not existing:
        call("/strategies", case["definition"])
    snapshot = call("/strategies/" + case["id"] + "/snapshot")
    report = {"equity_curve": True, "orders": True, "fills": True, "ledger": True,
              "metrics": ["final_equity", "total_return_pct", "max_drawdown_pct", "order_count", "fill_count", "max_position_leverage"]}
    response = call("/backtest-runs/" + existing["id"] + "/result") if existing else call(
        "/backtests", {"strategy_id": case["id"], "strategy_revision": snapshot["strategy_version"]["revision"],
                      "run_config": snapshot["default_run_config"], "report": report})
    comparable = copy.deepcopy(response["report"])
    comparable.pop("ledger", None)
    components = {key: sha(comparable[key]) for key in case["after"]["component_sha256"]}
    assert components == case["after"]["component_sha256"], (case["id"], components, case["after"]["component_sha256"])
    assert sha(comparable) == case["after"]["report_sha256"]
    assert response["strategy_version"]["definition_sha256"] == case["after"]["strategy_version"]["definition_sha256"]
    audit = audit_ledger(response)
    archived = call("/backtest-runs/" + response["run_id"] + "/result")
    stored_input = call("/backtest-runs/" + response["run_id"] + "/snapshot")
    assert archived == response and len(stored_input["data"]["bars"]) == response["bar_count"]
    item = {"id": case["id"], "run_id": response["run_id"], "definition": case["definition"],
            "strategy_version": response["strategy_version"], "data": response["data"], "run_config": response["run_config"],
            "report_sha256": sha(comparable), "component_sha256": components, "ledger_audit": audit,
            "order_count": response["order_count"], "fill_count": response["fill_count"],
            "final_equity": response["final_equity"], "baseline_equal": True, "archive_equal": True}
    if case["id"].startswith("verify_history"):
        by_time = {point["time_ms"]: point for point in response["report"]["equity_curve"]}
        checked = 0
        for order in response["report"]["orders"]:
            previous = by_time.get(order["signal_time_ms"] - 900_000)
            assert previous is not None and previous["position_qty"] == 0
            checked += 1
        item["historical_snapshot_audit"] = {"audited_orders": checked, "passed": True}
        evidence["history"].append(item)
    else:
        evidence["cases"].append(item)
    print(case["id"], "equal; ledger", audit["events"], flush=True)

# Isolated environment experiment: retain the original revision while changing
# initial cash and risk limits, then compare with the unchanged default run.
case = baseline["cases"][0]
snapshot = call("/strategies/" + case["id"] + "/snapshot")
original = copy.deepcopy(snapshot)
config = snapshot["default_run_config"]
config["account"]["initial_cash"] = 25000
config["risk"]["max_symbol_equity_pct"] = 0.1
config["risk"]["max_total_exposure_pct"] = 0.15
report = {"equity_curve": True, "orders": True, "fills": True, "ledger": True,
          "metrics": ["final_equity", "total_return_pct", "max_drawdown_pct", "order_count", "fill_count", "max_position_leverage"]}
existing = next((run for run in stored_runs if run["strategy_id"] == case["id"]
                 and run["status"] == "completed"
                 and run["request"]["run_config"] == config), None)
experiment = call("/backtest-runs/" + existing["id"] + "/result") if existing else call(
    "/backtests", {"strategy_id": case["id"], "strategy_revision": original["strategy_version"]["revision"],
                   "run_config": config, "report": report})
latest = call("/strategies/" + case["id"] + "/snapshot")
assert latest == original and experiment["strategy"]["initial_cash"] == 25000
assert experiment["run_config"]["risk"]["max_symbol_equity_pct"] == 0.1
admission = []
for event in experiment["report"]["ledger"]:
    if event["action"] != "close":
        # V1 limits admission against pre-fee equity. Market drift after the
        # fill can change exposure; this policy does not forcibly rebalance.
        price = event["fill"]["price"]
        equity = event["cash"] + event["position_qty"] * (price - event["average_price"]) + event["fill"]["fee"]
        admission.append(abs(event["position_qty"]) * price / equity)
assert max(admission) <= 0.1 + 1e-10
evidence["independent_config_experiment"] = {"run_id": experiment["run_id"], "strategy_version": experiment["strategy_version"],
    "run_config": experiment["run_config"], "strategy_snapshot_unchanged": True, "ledger_audit": audit_ledger(experiment),
    "final_equity": experiment["final_equity"], "max_admitted_exposure_ratio": max(admission),
    "risk_semantics": "admission cap against pre-fee equity; later mark-price drift does not force rebalancing",
    "max_position_leverage": max(point["position_leverage"] for point in experiment["report"]["equity_curve"])}
evidence["checks"] = {"real_market_baselines_equal": len(evidence["cases"]),
                      "historical_snapshot_orders_audited": sum(item["historical_snapshot_audit"]["audited_orders"] for item in evidence["history"]),
                      "ledger_events_audited": sum(item["ledger_audit"]["events"] for item in evidence["cases"] + evidence["history"]),
                      "archived_responses_equal": len(evidence["cases"] + evidence["history"])}
(ROOT / "strategy-architecture-completion-validation.json").write_text(json.dumps(evidence, ensure_ascii=False, indent=2) + "\n")
print(json.dumps(evidence["checks"]), flush=True)
