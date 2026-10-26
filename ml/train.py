#!/usr/bin/env python3

import argparse
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

import numpy as np
import pandas as pd
from sklearn.ensemble import HistGradientBoostingRegressor, RandomForestRegressor
from sklearn.linear_model import Ridge
from sklearn.metrics import mean_absolute_error, mean_squared_error, r2_score
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import StandardScaler

FEATURES = ["in_flight", "ema_ms", "cpu"]   # order matters, Go must use the same
MODEL_VERSION = 1
H_CONN, H_RT, H_CPU = 0.45, 0.40, 0.15      # current hand-tuned weights, for comparison


def load_data(data_dir, min_samples):
    paths = sorted(Path(data_dir).glob("*.csv"))
    if not paths:
        sys.exit("No CSV files in %s. Run collect_data.py first." % data_dir)
    df = pd.concat([pd.read_csv(p) for p in paths], ignore_index=True)
    n_all = len(df)
    df = df[df["status"] == 200].copy()
    df = df[df["ema_samples"] >= min_samples].copy()
    df["group"] = df["run_id"].astype(str) + "-" + df["regime_id"].astype(str)
    df["cpu_raw"] = df["cpu"]
    print("Loaded %d files, %d rows (%d usable after filtering)" % (len(paths), n_all, len(df)))
    return df


def fill_missing(df):
    defaults = {
        "in_flight": 0.0,
        "ema_ms": float(df["ema_ms"].median()),
        "cpu": float(df["cpu"].median()),
    }
    df["cpu"] = df["cpu"].fillna(defaults["cpu"])
    return defaults


def split_by_group(df, test_frac, seed):
    groups = np.array(sorted(df["group"].unique().tolist()))
    rng = np.random.RandomState(seed)
    rng.shuffle(groups)
    n_test = max(1, int(len(groups) * test_frac))
    mask = df["group"].isin(set(groups[:n_test]))
    return df[~mask].copy(), df[mask].copy()


def build_models(seed):
    return {
        "linear_ridge": make_pipeline(StandardScaler(), Ridge(alpha=1.0)),
        "random_forest": RandomForestRegressor(
            n_estimators=150, min_samples_leaf=5, n_jobs=-1, random_state=seed),
        "gradient_boosting": HistGradientBoostingRegressor(
            max_iter=250, learning_rate=0.08, random_state=seed),
    }


def regression_metrics(y_true, y_pred):
    return {
        "mae_ms": float(mean_absolute_error(y_true, y_pred)),
        "rmse_ms": float(np.sqrt(mean_squared_error(y_true, y_pred))),
        "r2": float(r2_score(y_true, y_pred)),
    }


def heuristic_pick(group):
    """Approximation of the current Go Adaptive.NextServer."""
    def minmax(s):
        lo, hi = s.min(), s.max()
        return pd.Series(0.0, index=s.index) if hi <= lo else (s - lo) / (hi - lo)

    conn = minmax(group["in_flight"].astype(float))
    rt = minmax(group["ema_ms"].astype(float))
    cpu = (group["cpu_raw"] / 100.0).clip(0, 1).fillna(0.5)
    score = H_CONN * conn + H_RT * rt + H_CPU * cpu
    cand = group[score <= score.min() + 1e-6]
    return cand["static_weight"].idxmax()


def routing_test(test_df, models, seed, n_servers):
    probes = test_df[test_df["is_probe"] == 1]
    groups = [g for _, g in probes.groupby(["run_id", "probe_id"]) if len(g) == n_servers]
    if not groups:
        return None, 0

    rng = np.random.RandomState(seed)
    picked = {k: [] for k in ["oracle", "random", "least_connections", "hand_tuned_adaptive"]}
    for name in models:
        picked["ml_" + name] = []

    all_rows = pd.concat(groups)
    preds = {name: pd.Series(m.predict(all_rows[FEATURES].values), index=all_rows.index)
             for name, m in models.items()}

    for g in groups:
        lat = g["latency_ms"]
        picked["oracle"].append(lat.min())
        picked["random"].append(lat.iloc[rng.randint(len(g))])
        mins = g[g["in_flight"] == g["in_flight"].min()]
        picked["least_connections"].append(mins["latency_ms"].iloc[rng.randint(len(mins))])
        picked["hand_tuned_adaptive"].append(lat[heuristic_pick(g)])
        for name in models:
            picked["ml_" + name].append(lat[preds[name][g.index].idxmin()])

    out = {}
    for k, v in picked.items():
        v = np.array(v)
        out[k] = {"mean_ms": float(v.mean()), "p95_ms": float(np.percentile(v, 95))}
    return out, len(groups)


def export_linear(pipe, defaults, metrics, n_rows, path, routing):
    scaler, ridge = pipe.steps[0][1], pipe.steps[1][1]
    # fold the scaler in so Go works on RAW values: pred = intercept + sum(coef*x)
    coef = ridge.coef_ / scaler.scale_
    intercept = float(ridge.intercept_ - np.sum(ridge.coef_ * scaler.mean_ / scaler.scale_))
    model = {
        "version": MODEL_VERSION,
        "type": "linear",
        "target": "predicted_latency_ms (lower is better, pick the minimum)",
        "features": FEATURES,
        "coefficients": [float(c) for c in coef],
        "intercept": intercept,
        "defaults": defaults,
        "trained_at": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "training_rows": int(n_rows),
        "test_metrics": metrics,
        "routing_test_mean_ms": {k: round(v["mean_ms"], 3) for k, v in (routing or {}).items()},
        "notes": [
            "in_flight = count before adding the new request, ema_ms = milliseconds, cpu = percent 0-100.",
            "If a server has no response samples yet, use defaults.ema_ms.",
            "If CPU is stale or missing, use defaults.cpu.",
        ],
    }
    Path(path).parent.mkdir(parents=True, exist_ok=True)
    with open(path, "w") as f:
        json.dump(model, f, indent=2)
    return model


def check_export(model, pipe, test_df):
    X = test_df[FEATURES].values
    manual = model["intercept"] + X @ np.array(model["coefficients"])
    if not np.allclose(manual, pipe.predict(X), atol=1e-6):
        sys.exit("EXPORT CHECK FAILED: model.json does not match the sklearn model")
    print("Export check passed: model.json reproduces the sklearn predictions.")


def make_plots(test_df, models, routing, out_dir):
    try:
        import matplotlib
        matplotlib.use("Agg")
        import matplotlib.pyplot as plt
    except Exception:
        print("matplotlib not available, skipping plots")
        return
    out_dir = Path(out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    sample = test_df.sample(min(3000, len(test_df)), random_state=0)
    fig, axes = plt.subplots(1, len(models), figsize=(5 * len(models), 4.5))
    axes = np.atleast_1d(axes)
    lim = float(np.percentile(sample["latency_ms"], 99))
    for ax, (name, m) in zip(axes, models.items()):
        ax.scatter(sample["latency_ms"], m.predict(sample[FEATURES].values), s=4, alpha=0.3)
        ax.plot([0, lim], [0, lim], "r--", lw=1)
        ax.set_xlim(0, lim)
        ax.set_ylim(0, lim)
        ax.set_title(name)
        ax.set_xlabel("actual latency (ms)")
        ax.set_ylabel("predicted latency (ms)")
    fig.tight_layout()
    fig.savefig(out_dir / "predicted_vs_actual.png", dpi=130)
    plt.close(fig)

    if routing:
        names = list(routing.keys())
        fig, ax = plt.subplots(figsize=(9, 4.5))
        ax.barh(names, [routing[n]["mean_ms"] for n in names])
        ax.invert_yaxis()
        ax.set_xlabel("mean latency of the picked server (ms), lower is better")
        ax.set_title("Routing test on held-out probes")
        fig.tight_layout()
        fig.savefig(out_dir / "routing_test.png", dpi=130)
        plt.close(fig)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--data", default="data/raw")
    ap.add_argument("--out", default="model/model.json")
    ap.add_argument("--reports", default="reports")
    ap.add_argument("--min-samples", type=int, default=5)
    ap.add_argument("--test-frac", type=float, default=0.25)
    ap.add_argument("--seed", type=int, default=42)
    args = ap.parse_args()

    df = load_data(args.data, args.min_samples)
    defaults = fill_missing(df)
    n_servers = df["server_id"].nunique()
    train_df, test_df = split_by_group(df, args.test_frac, args.seed)
    print("Train rows: %d   Test rows: %d   Servers: %d" % (len(train_df), len(test_df), n_servers))

    models = build_models(args.seed)
    metrics = {}
    baseline = regression_metrics(test_df["latency_ms"],
                                  np.full(len(test_df), train_df["latency_ms"].mean()))
    print("\nPrediction error on held-out regimes")
    print("%-20s %10s %10s %8s" % ("model", "MAE (ms)", "RMSE (ms)", "R2"))
    print("%-20s %10.2f %10.2f %8.3f" % ("predict-the-mean", baseline["mae_ms"], baseline["rmse_ms"], baseline["r2"]))
    for name, m in models.items():
        m.fit(train_df[FEATURES].values, train_df["latency_ms"].values)
        metrics[name] = regression_metrics(test_df["latency_ms"], m.predict(test_df[FEATURES].values))
        r = metrics[name]
        print("%-20s %10.2f %10.2f %8.3f" % (name, r["mae_ms"], r["rmse_ms"], r["r2"]))

    routing, n_probe_groups = routing_test(test_df, models, args.seed, n_servers)
    if routing:
        print("\nRouting test on %d held-out probe rounds (latency of the server each policy picks)" % n_probe_groups)
        print("%-26s %10s %10s" % ("policy", "mean (ms)", "p95 (ms)"))
        for k, v in sorted(routing.items(), key=lambda kv: kv[1]["mean_ms"]):
            print("%-26s %10.2f %10.2f" % (k, v["mean_ms"], v["p95_ms"]))
        print("(oracle = best server in hindsight, a lower bound nobody can reach)")
    else:
        print("\nNo complete probe rounds in the test set, skipping the routing test.")

    final = build_models(args.seed)["linear_ridge"]
    final.fit(df[FEATURES].values, df["latency_ms"].values)
    model = export_linear(final, defaults, metrics["linear_ridge"], len(df), args.out, routing)
    check_export(model, final, test_df)

    print("\nLearned linear model (raw units):")
    print("  predicted_ms = %.3f" % model["intercept"])
    for f, c in zip(FEATURES, model["coefficients"]):
        print("               + %.4f * %s" % (c, f))
    print("Saved %s" % args.out)

    Path(args.reports).mkdir(parents=True, exist_ok=True)
    with open(Path(args.reports) / "metrics.json", "w") as f:
        json.dump({"regression": metrics, "baseline_mean": baseline, "routing": routing,
                   "probe_rounds": n_probe_groups, "train_rows": len(train_df),
                   "test_rows": len(test_df)}, f, indent=2)
    make_plots(test_df, models, routing, args.reports)


if __name__ == "__main__":
    main()