#!/usr/bin/env python3
"""
Data collector for the ML-driven adaptive load balancer.
Uses persistent HTTP connection pooling to prevent Windows TCP port exhaustion.
"""

import argparse
import csv
import random
import signal
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from urllib.parse import urlparse
import requests
from requests.adapters import HTTPAdapter

EMA_ALPHA = 0.2
COLUMNS = [
    "ts", "run_id", "regime_id", "regime_age_s", "probe_id", "is_probe",
    "server_id", "static_weight",
    "in_flight", "ema_ms", "ema_samples", "cpu", "cpu_age_s",
    "latency_ms", "status",
    "cfg_latency_ms", "cfg_cpu_workers", "rate_rps"
]

class Backend:
    def __init__(self, bid, url, weight):
        self.id = bid
        self.url = url.rstrip("/")
        self.weight = weight
        self.lock = threading.Lock()
        self.in_flight = 0
        self.ema = 0.0
        self.samples = 0
        self.cpu = None
        self.cpu_at = None
        self.cfg_latency = 0
        self.cfg_workers = 0

    def begin(self):
        with self.lock:
            snap = {
                "in_flight": self.in_flight,
                "ema_ms": self.ema,
                "ema_samples": self.samples,
                "cpu": self.cpu,
                "cpu_age_s": (time.monotonic() - self.cpu_at) if self.cpu_at else None,
            }
            self.in_flight += 1
        return snap

    def end(self, latency_ms):
        with self.lock:
            if self.in_flight > 0:
                self.in_flight -= 1
            if latency_ms is not None:
                if self.samples == 0:
                    self.ema = latency_ms
                else:
                    self.ema = EMA_ALPHA * latency_ms + (1.0 - EMA_ALPHA) * self.ema
                self.samples += 1

    def set_cpu(self, value):
        with self.lock:
            self.cpu = value
            self.cpu_at = time.monotonic()


class Collector:
    def __init__(self, args):
        urls = [u.strip() for u in args.backends.split(",") if u.strip()]
        ids = [i.strip() for i in args.ids.split(",") if i.strip()]
        weights = [int(w) for w in args.weights.split(",") if w.strip()]
        if not (len(urls) == len(ids) == len(weights)):
            raise SystemExit("--backends, --ids and --weights length mismatch.")
        self.backends = [Backend(i, u, w) for i, u, w in zip(ids, urls, weights)]
        self.args = args
        self.rng = random.Random(args.seed)
        self.stop = threading.Event()
        self.t_start = time.monotonic()
        self.run_id = int(time.time())
        self.regime_id = 0
        self.regime_started = time.monotonic()
        self.rate = 0.0
        self.probe_id = 0
        self.rows_written = 0
        self.errors = 0
        self.write_lock = threading.Lock()

        # Thread-local storage for persistent HTTP sessions
        self.thread_local = threading.local()

        out = Path(args.out) if args.out else Path("data/raw") / f"run_{self.run_id}.csv"
        out.parent.mkdir(parents=True, exist_ok=True)
        self.out_path = out
        self.fh = open(out, "w", newline="", encoding="utf-8")
        self.writer = csv.writer(self.fh)
        self.writer.writerow(COLUMNS)
        self.fh.flush()

    def get_session(self):
        if not hasattr(self.thread_local, "session"):
            session = requests.Session()
            adapter = HTTPAdapter(pool_connections=10, pool_maxsize=20, max_retries=1)
            session.mount("http://", adapter)
            session.mount("https://", adapter)
            self.thread_local.session = session
        return self.thread_local.session

    def write_row(self, row):
        with self.write_lock:
            self.writer.writerow(row)
            self.rows_written += 1
            if self.rows_written % 100 == 0:
                self.fh.flush()

    def send(self, backend, is_probe, probe_id):
        snap = backend.begin()
        regime_id = self.regime_id
        regime_age = time.monotonic() - self.regime_started
        cfg_lat, cfg_workers, rate = backend.cfg_latency, backend.cfg_workers, self.rate
        
        session = self.get_session()
        t0 = time.perf_counter()
        status = 0
        try:
            resp = session.get(f"{backend.url}/", timeout=self.args.timeout)
            status = resp.status_code
        except Exception:
            status = 0
        latency_ms = (time.perf_counter() - t0) * 1000.0
        ok = (status == 200)
        backend.end(latency_ms if ok else None)
        if not ok:
            self.errors += 1

        def fmt(v, nd=3):
            return "" if v is None else round(v, nd)

        self.write_row([
            round(time.time(), 3), self.run_id, regime_id, round(regime_age, 2),
            probe_id, int(is_probe), backend.id, backend.weight,
            snap["in_flight"], fmt(snap["ema_ms"]), snap["ema_samples"],
            fmt(snap["cpu"]), fmt(snap["cpu_age_s"], 2),
            round(latency_ms, 3), status,
            cfg_lat, cfg_workers, rate
        ])

    def cpu_poller(self):
        session = requests.Session()
        while not self.stop.is_set():
            for b in self.backends:
                try:
                    resp = session.get(f"{b.url}/metrics/cpu", timeout=2.0)
                    if resp.status_code == 200:
                        b.set_cpu(float(resp.json()["cpu_utilization"]))
                except Exception:
                    pass
            self.stop.wait(self.args.cpu_poll)

    def regime_controller(self):
        a = self.args
        lat_choices = [int(x) for x in a.latencies.split(",")]
        cpu_choices = [int(x) for x in a.cpu_workers.split(",")]
        rate_choices = [float(x) for x in a.rates.split(",")]
        session = requests.Session()
        
        while not self.stop.is_set():
            self.regime_id += 1
            self.regime_started = time.monotonic()
            self.rate = self.rng.choice(rate_choices)
            for b in self.backends:
                lat = self.rng.choice(lat_choices)
                workers = self.rng.choice(cpu_choices)
                try:
                    session.post(f"{b.url}/bench/latency?ms={lat}", timeout=2.0)
                    session.post(f"{b.url}/bench/cpu?workers={workers}", timeout=2.0)
                    b.cfg_latency, b.cfg_workers = lat, workers
                except Exception as exc:
                    print(f"Failed to reconfigure {b.id}: {exc}", flush=True)
            self.stop.wait(a.regime_s)

    def probe_loop(self, pool):
        while not self.stop.is_set():
            self.stop.wait(self.args.probe_every)
            if self.stop.is_set():
                break
            self.probe_id += 1
            pid = self.probe_id
            for b in self.backends:
                pool.submit(self.send, b, True, pid)

    def load_loop(self, pool):
        next_t = time.monotonic()
        while not self.stop.is_set():
            rate = self.rate
            if rate <= 0:
                self.stop.wait(0.1)
                next_t = time.monotonic()
                continue
            next_t += self.rng.expovariate(rate)
            delay = next_t - time.monotonic()
            if delay > 0:
                if self.stop.wait(delay):
                    break
            elif delay < -1.0:
                next_t = time.monotonic()
            pool.submit(self.send, self.rng.choice(self.backends), False, 0)

    def run(self):
        a = self.args
        print(f"Connecting to backends and starting collection ({a.duration}s)...", flush=True)
        print(f"Output destination: {self.out_path}", flush=True)

        session = requests.Session()
        for b in self.backends:
            try:
                resp = session.get(f"{b.url}/health", timeout=2.0)
                print(f"  Backend {b.id} ({b.url}) health-check passed (HTTP {resp.status_code})", flush=True)
            except Exception as exc:
                print(f"\n[ERROR] Cannot reach backend {b.id} at {b.url} ({exc}).", flush=True)
                sys.exit(1)

        pool = ThreadPoolExecutor(max_workers=a.max_workers)
        threads = [
            threading.Thread(target=self.cpu_poller, daemon=True),
            threading.Thread(target=self.regime_controller, daemon=True),
            threading.Thread(target=self.probe_loop, args=(pool,), daemon=True),
            threading.Thread(target=self.load_loop, args=(pool,), daemon=True),
        ]
        for t in threads:
            t.start()

        end = time.monotonic() + a.duration
        try:
            while time.monotonic() < end and not self.stop.is_set():
                time.sleep(min(5.0, max(0.1, end - time.monotonic())))
                elapsed = time.monotonic() - self.t_start
                print(f"  t={elapsed:4.0f}s | regime={self.regime_id} | rows={self.rows_written} | errors={self.errors}", flush=True)
        except KeyboardInterrupt:
            print("\nAborting collection early...", flush=True)

        self.stop.set()
        pool.shutdown(wait=True)

        # Reset backends to baseline state
        for b in self.backends:
            try:
                session.post(f"{b.url}/bench/latency?ms=0", timeout=2.0)
                session.post(f"{b.url}/bench/cpu?workers=0", timeout=2.0)
            except Exception:
                pass

        self.fh.flush()
        self.fh.close()
        print(f"Done. Successfully wrote {self.rows_written} rows ({self.errors} errors) to {self.out_path}", flush=True)

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--backends", default="http://127.0.0.1:18081,http://127.0.0.1:18082,http://127.0.0.1:18083")
    p.add_argument("--ids", default="backend-1,backend-2,backend-3")
    p.add_argument("--weights", default="3,1,2")
    p.add_argument("--duration", type=int, default=600)
    p.add_argument("--regime-s", type=float, default=20.0)
    p.add_argument("--latencies", default="5,20,50,100,200")
    p.add_argument("--cpu-workers", default="0,0,0,1,2")
    # Reduced max concurrency rate choices slightly for local OS socket stability:
    p.add_argument("--rates", default="40,80,120,180")
    p.add_argument("--probe-every", type=float, default=0.5)
    p.add_argument("--cpu-poll", type=float, default=5.0)
    p.add_argument("--timeout", type=float, default=5.0)
    p.add_argument("--max-workers", type=int, default=64)
    p.add_argument("--seed", type=int, default=42)
    p.add_argument("--out", default=None)
    args = p.parse_args()

    c = Collector(args)
    if hasattr(signal, "SIGINT"):
        signal.signal(signal.SIGINT, lambda *_: c.stop.set())
    if hasattr(signal, "SIGTERM"):
        try:
            signal.signal(signal.SIGTERM, lambda *_: c.stop.set())
        except Exception:
            pass

    c.run()

if __name__ == "__main__":
    main()