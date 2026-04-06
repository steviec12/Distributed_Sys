#!/usr/bin/env python3

import argparse
import csv
import json
import os
import random
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from collections import deque
from datetime import datetime

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
HW10_DIR = os.path.dirname(SCRIPT_DIR)


def request_json(method, url, payload=None, timeout=3.0):
    data = None
    headers = {}
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"

    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    started_at = time.time()
    started_perf = time.perf_counter()

    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            raw_body = response.read().decode("utf-8")
            try:
                body = json.loads(raw_body) if raw_body else None
            except json.JSONDecodeError:
                body = raw_body
            return {
                "status": response.getcode(),
                "body": body,
                "error": None,
                "started_at_ms": round(started_at * 1000, 3),
                "elapsed_ms": round((time.perf_counter() - started_perf) * 1000, 3),
            }
    except urllib.error.HTTPError as err:
        raw_body = err.read().decode("utf-8")
        try:
            body = json.loads(raw_body) if raw_body else None
        except json.JSONDecodeError:
            body = raw_body
        return {
            "status": err.code,
            "body": body,
            "error": None,
            "started_at_ms": round(started_at * 1000, 3),
            "elapsed_ms": round((time.perf_counter() - started_perf) * 1000, 3),
        }
    except Exception as err:  # noqa: BLE001
        return {
            "status": None,
            "body": None,
            "error": str(err),
            "started_at_ms": round(started_at * 1000, 3),
            "elapsed_ms": round((time.perf_counter() - started_perf) * 1000, 3),
        }


def percentile(values, pct):
    if not values:
        return None
    ordered = sorted(values)
    index = int(round((pct / 100.0) * (len(ordered) - 1)))
    return round(ordered[index], 3)


def parse_ratio(text):
    parts = text.split("/")
    if len(parts) != 2:
        raise ValueError(f"invalid ratio {text!r}, expected READ/WRITE like 90/10")
    read_pct = float(parts[0])
    write_pct = float(parts[1])
    total = read_pct + write_pct
    if total <= 0:
        raise ValueError("ratio total must be positive")
    return read_pct / total, write_pct / total


class SharedState:
    def __init__(self, recent_capacity):
        self.lock = threading.Lock()
        self.latest_writes = {}
        self.write_counter = 0
        self.recent_keys = deque(maxlen=recent_capacity)

    def next_value(self, key):
        with self.lock:
            self.write_counter += 1
            return f"{key}-value-{self.write_counter}"

    def update_write(self, key, version, ack_time_ms, coordinator_node):
        with self.lock:
            self.latest_writes[key] = {
                "version": version,
                "ack_time_ms": ack_time_ms,
                "coordinator_node": coordinator_node,
            }
            self.recent_keys.append(key)

    def latest_before(self, key, read_start_ms):
        with self.lock:
            info = self.latest_writes.get(key)
            if info is None:
                return None
            if info["ack_time_ms"] <= read_start_ms:
                return dict(info)
            return None

    def choose_recent_key(self, rng):
        with self.lock:
            if not self.recent_keys:
                return None
            return rng.choice(list(self.recent_keys))


class Recorder:
    def __init__(self, output_dir):
        self.output_dir = output_dir
        os.makedirs(output_dir, exist_ok=True)
        self.csv_path = os.path.join(output_dir, "requests.csv")
        self.summary_path = os.path.join(output_dir, "summary.json")
        self.config_path = os.path.join(output_dir, "config.json")
        self.csv_file = open(self.csv_path, "w", newline="", encoding="utf-8")
        self.writer = csv.DictWriter(
            self.csv_file,
            fieldnames=[
                "started_at_ms",
                "op",
                "node",
                "url",
                "key",
                "status",
                "latency_ms",
                "expected_version",
                "observed_version",
                "stale",
                "stale_eligible",
                "target_kind",
                "read_after_write_ms",
                "error",
            ],
        )
        self.writer.writeheader()
        self.lock = threading.Lock()
        self.read_latencies = []
        self.write_latencies = []
        self.stale_reads = 0
        self.total_reads = 0
        self.total_writes = 0
        self.eligible_reads = 0
        self.read_after_write_intervals = []

    def record(self, row):
        with self.lock:
            self.writer.writerow(row)
            self.csv_file.flush()

            latency = row["latency_ms"]
            if row["op"] == "read":
                self.total_reads += 1
                if latency is not None:
                    self.read_latencies.append(float(latency))
                if row["stale_eligible"]:
                    self.eligible_reads += 1
                if row["stale"]:
                    self.stale_reads += 1
                if row["read_after_write_ms"] not in ("", None):
                    self.read_after_write_intervals.append(float(row["read_after_write_ms"]))
            else:
                self.total_writes += 1
                if latency is not None:
                    self.write_latencies.append(float(latency))

    def write_config(self, config):
        with open(self.config_path, "w", encoding="utf-8") as handle:
            json.dump(config, handle, indent=2)

    def close(self):
        self.csv_file.close()

    def write_summary(self):
        summary = {
            "total_reads": self.total_reads,
            "total_writes": self.total_writes,
            "eligible_reads_for_staleness": self.eligible_reads,
            "stale_reads_on_followers_or_non_coordinators": self.stale_reads,
            "stale_read_rate_on_eligible_reads": round(self.stale_reads / self.eligible_reads, 6) if self.eligible_reads else None,
            "read_latency_ms": {
                "count": len(self.read_latencies),
                "p50": percentile(self.read_latencies, 50),
                "p95": percentile(self.read_latencies, 95),
                "p99": percentile(self.read_latencies, 99),
            },
            "write_latency_ms": {
                "count": len(self.write_latencies),
                "p50": percentile(self.write_latencies, 50),
                "p95": percentile(self.write_latencies, 95),
                "p99": percentile(self.write_latencies, 99),
            },
            "read_after_write_ms": {
                "count": len(self.read_after_write_intervals),
                "p50": percentile(self.read_after_write_intervals, 50),
                "p95": percentile(self.read_after_write_intervals, 95),
                "p99": percentile(self.read_after_write_intervals, 99),
            },
        }
        with open(self.summary_path, "w", encoding="utf-8") as handle:
            json.dump(summary, handle, indent=2)
        return summary


def choose_write_target(mode, leader_url, node_urls, rng):
    if mode == "leader_follower":
        return leader_url
    return rng.choice(node_urls)


def choose_key_for_read(keys, rng, state, recent_key_bias):
    if rng.random() < recent_key_bias:
        recent = state.choose_recent_key(rng)
        if recent is not None:
            return recent
    return rng.choice(keys)


def choose_read_target(mode, leader_url, node_urls, node_names, key, rng, state):
    if mode == "leader_follower":
        follower_urls = [url for url in node_urls if url != leader_url]
        if follower_urls:
            target_url = rng.choice(follower_urls)
            return target_url, "follower"
        target_url = rng.choice(node_urls)
        return target_url, "leader"

    latest = state.latest_before(key, float("inf"))
    if latest is not None:
        coordinator_node = latest.get("coordinator_node")
        candidates = [url for url in node_urls if node_names[url] != coordinator_node]
        if candidates:
            return rng.choice(candidates), "non_coordinator"
    return rng.choice(node_urls), "unknown"


def perform_write(mode, leader_url, node_urls, node_names, key, timeout, rng, state, recorder=None):
    target_url = choose_write_target(mode, leader_url, node_urls, rng)
    node_name = node_names[target_url]
    value = state.next_value(key)
    result = request_json("POST", f"{target_url}/set", payload={"key": key, "value": value}, timeout=timeout)

    observed_version = None
    if result["status"] == 201 and isinstance(result["body"], dict):
        observed_version = result["body"].get("version")
        if observed_version is not None:
            ack_time_ms = result["started_at_ms"] + result["elapsed_ms"]
            state.update_write(key, observed_version, ack_time_ms, node_name)

    if recorder is not None:
        recorder.record(
            {
                "started_at_ms": result["started_at_ms"],
                "op": "write",
                "node": node_name,
                "url": target_url,
                "key": key,
                "status": result["status"],
                "latency_ms": result["elapsed_ms"],
                "expected_version": "",
                "observed_version": observed_version if observed_version is not None else "",
                "stale": "",
                "read_after_write_ms": "",
                "error": result["error"] or "",
            }
        )


def perform_read(mode, leader_url, node_urls, node_names, key, timeout, rng, state, recorder):
    target_url, target_kind = choose_read_target(mode, leader_url, node_urls, node_names, key, rng, state)
    node_name = node_names[target_url]
    result = request_json("GET", f"{target_url}/get?{urllib.parse.urlencode({'key': key})}", timeout=timeout)

    latest = state.latest_before(key, result["started_at_ms"])
    expected_version = latest["version"] if latest else None
    read_after_write_ms = None
    if latest is not None:
        read_after_write_ms = round(result["started_at_ms"] - latest["ack_time_ms"], 3)

    observed_version = None
    if result["status"] == 200 and isinstance(result["body"], dict):
        observed_version = result["body"].get("version")

    stale = False
    stale_eligible = False
    if expected_version is not None:
        stale_eligible = target_kind in {"follower", "non_coordinator"}
        if result["status"] != 200:
            stale = True
        elif observed_version is None or observed_version < expected_version:
            stale = True
        if not stale_eligible:
            stale = False

    recorder.record(
        {
            "started_at_ms": result["started_at_ms"],
            "op": "read",
            "node": node_name,
            "url": target_url,
            "key": key,
            "status": result["status"],
            "latency_ms": result["elapsed_ms"],
            "expected_version": expected_version if expected_version is not None else "",
            "observed_version": observed_version if observed_version is not None else "",
            "stale": int(stale) if expected_version is not None else "",
            "stale_eligible": int(stale_eligible) if expected_version is not None else "",
            "target_kind": target_kind,
            "read_after_write_ms": read_after_write_ms if read_after_write_ms is not None else "",
            "error": result["error"] or "",
        }
    )


def worker_loop(worker_id, mode, leader_url, node_urls, node_names, keys, timeout, read_probability, recent_key_bias, stop_event, state, recorder, seed):
    rng = random.Random(seed + worker_id)
    while not stop_event.is_set():
        if rng.random() < read_probability:
            key = choose_key_for_read(keys, rng, state, recent_key_bias)
            perform_read(mode, leader_url, node_urls, node_names, key, timeout, rng, state, recorder)
        else:
            key = rng.choice(keys)
            perform_write(mode, leader_url, node_urls, node_names, key, timeout, rng, state, recorder)


def preseed(mode, leader_url, node_urls, node_names, keys, timeout, state, recorder):
    rng = random.Random(0)
    for key in keys:
        perform_write(mode, leader_url, node_urls, node_names, key, timeout, rng, state, recorder=None)


def main():
    parser = argparse.ArgumentParser(
        description=(
            "Run the hw10 load tester. "
            "It generates a configurable read/write mix, logs every request to CSV, and counts stale public reads "
            "relative to the latest acknowledged write for each key."
        )
    )
    parser.add_argument("--mode", choices=["leader_follower", "leaderless"], required=True)
    parser.add_argument("--nodes", required=True, help="Comma-separated public node URLs in node1..node5 order.")
    parser.add_argument("--leader", help="Leader URL for leader_follower mode.")
    parser.add_argument("--ratio", required=True, help="Read/write ratio like 90/10.")
    parser.add_argument("--workers", type=int, default=20, help="Number of concurrent worker threads.")
    parser.add_argument("--duration-s", type=int, default=60, help="How long to run the workload.")
    parser.add_argument("--timeout", type=float, default=3.0, help="Per-request timeout in seconds.")
    parser.add_argument("--hot-keys", type=int, default=20, help="Number of keys to cycle through.")
    parser.add_argument("--preseed", type=int, default=20, help="How many keys to seed before starting the workload.")
    parser.add_argument("--recent-key-bias", type=float, default=0.85, help="Probability that a read picks from recently written keys.")
    parser.add_argument("--recent-window", type=int, default=100, help="How many recent acknowledged writes to keep for local-in-time read selection.")
    parser.add_argument("--output-dir", help="Directory for requests.csv and summary.json.")
    parser.add_argument("--seed", type=int, default=42, help="Random seed.")
    args = parser.parse_args()

    node_urls = [url.strip() for url in args.nodes.split(",") if url.strip()]
    if len(node_urls) != 5:
        raise SystemExit("--nodes must contain exactly 5 URLs in node1..node5 order")
    if args.mode == "leader_follower" and not args.leader:
        raise SystemExit("--leader is required for leader_follower mode")

    read_probability, _ = parse_ratio(args.ratio)
    keys = [f"key-{index:04d}" for index in range(args.hot_keys)]
    preseed_keys = keys[: min(args.preseed, len(keys))]

    timestamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    output_dir = args.output_dir or os.path.join(HW10_DIR, "results", f"{args.mode}-{args.ratio.replace('/', '_')}-{timestamp}")
    recorder = Recorder(output_dir)
    recorder.write_config(
        {
            "mode": args.mode,
            "nodes": node_urls,
            "leader": args.leader,
            "ratio": args.ratio,
            "workers": args.workers,
            "duration_s": args.duration_s,
            "timeout": args.timeout,
            "hot_keys": args.hot_keys,
            "preseed": len(preseed_keys),
            "recent_key_bias": args.recent_key_bias,
            "recent_window": args.recent_window,
            "seed": args.seed,
        }
    )

    node_names = {url: f"node{index}" for index, url in enumerate(node_urls, start=1)}
    state = SharedState(args.recent_window)

    print(f"Mode:        {args.mode}")
    print(f"Nodes:       {', '.join(node_urls)}")
    print(f"Ratio:       {args.ratio} (read_probability={read_probability:.2f})")
    print(f"Workers:     {args.workers}")
    print(f"Duration:    {args.duration_s}s")
    print(f"Hot keys:    {args.hot_keys}")
    print(f"Preseed:     {len(preseed_keys)} keys")
    print(f"Recent bias: {args.recent_key_bias}")
    print(f"Recent win:  {args.recent_window}")
    print(f"Output dir:  {output_dir}")

    if preseed_keys:
        print("Preseeding keys")
        preseed(args.mode, args.leader, node_urls, node_names, preseed_keys, args.timeout, state, recorder)

    stop_event = threading.Event()
    threads = []
    for worker_id in range(args.workers):
        thread = threading.Thread(
            target=worker_loop,
            args=(
                worker_id,
                args.mode,
                args.leader,
                node_urls,
                node_names,
                keys,
                args.timeout,
                read_probability,
                args.recent_key_bias,
                stop_event,
                state,
                recorder,
                args.seed,
            ),
            daemon=True,
        )
        threads.append(thread)
        thread.start()

    time.sleep(args.duration_s)
    stop_event.set()
    for thread in threads:
        thread.join()

    summary = recorder.write_summary()
    recorder.close()

    print("\nSummary:")
    print(json.dumps(summary, indent=2))


if __name__ == "__main__":
    main()
