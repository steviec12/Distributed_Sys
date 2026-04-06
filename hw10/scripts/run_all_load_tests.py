#!/usr/bin/env python3

import argparse
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request


SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
HW10_DIR = os.path.dirname(SCRIPT_DIR)
DEPLOY_SCRIPT = os.path.join(SCRIPT_DIR, "deploy_ec2.sh")
LOAD_TEST_SCRIPT = os.path.join(SCRIPT_DIR, "load_test.py")
RESULTS_ROOT = os.path.join(HW10_DIR, "results")

NODE_URLS = [
    "http://54.186.92.177:8080",
    "http://54.184.82.165:8080",
    "http://35.90.20.230:8080",
    "http://52.36.35.254:8080",
    "http://34.220.181.249:8080",
]
LEADER_URL = NODE_URLS[0]

LEADER_FOLLOWER_RUNS = [
    {"mode": "leader_follower", "w": 5, "r": 1, "label": "leader_follower-w5-r1"},
    {"mode": "leader_follower", "w": 1, "r": 5, "label": "leader_follower-w1-r5"},
    {"mode": "leader_follower", "w": 3, "r": 3, "label": "leader_follower-w3-r3"},
]

RATIOS = [
    ("99/1", "99_1"),
    ("90/10", "90_10"),
    ("50/50", "50_50"),
    ("10/90", "10_90"),
]


def run_command(cmd, cwd=None):
    print(f"\n$ {' '.join(cmd)}")
    subprocess.run(cmd, cwd=cwd, check=True)


def fetch_health(url, timeout=5.0):
    request = urllib.request.Request(f"{url}/health", method="GET")
    with urllib.request.urlopen(request, timeout=timeout) as response:
        return json.loads(response.read().decode("utf-8"))


def verify_health(expected_mode, expected_w, expected_r):
    print("Verifying cluster health")
    for index, url in enumerate(NODE_URLS, start=1):
        body = fetch_health(url)
        expected_role = "leader" if expected_mode == "leader_follower" and index == 1 else (
            "follower" if expected_mode == "leader_follower" else "replica"
        )

        if body.get("mode") != expected_mode:
            raise RuntimeError(f"{url} mode mismatch: expected {expected_mode}, got {body.get('mode')}")
        if body.get("w") != expected_w:
            raise RuntimeError(f"{url} w mismatch: expected {expected_w}, got {body.get('w')}")
        if body.get("r") != expected_r:
            raise RuntimeError(f"{url} r mismatch: expected {expected_r}, got {body.get('r')}")
        if body.get("role") != expected_role:
            raise RuntimeError(f"{url} role mismatch: expected {expected_role}, got {body.get('role')}")

        print(f"  node{index}: ok ({body['mode']} w={body['w']} r={body['r']} role={body['role']})")


def deploy_mode(run):
    if run["mode"] == "leaderless":
        cmd = ["bash", DEPLOY_SCRIPT, "leaderless"]
    else:
        cmd = ["bash", DEPLOY_SCRIPT, "leader_follower", str(run["w"]), str(run["r"])]
    run_command(cmd)


def run_load_test(run, ratio, ratio_label, workers, duration_s):
    output_dir = os.path.join(RESULTS_ROOT, run["label"], ratio_label)
    os.makedirs(output_dir, exist_ok=True)

    cmd = [
        sys.executable,
        LOAD_TEST_SCRIPT,
        "--mode",
        run["mode"],
        "--nodes",
        ",".join(NODE_URLS),
        "--ratio",
        ratio,
        "--workers",
        str(workers),
        "--duration-s",
        str(duration_s),
        "--output-dir",
        output_dir,
    ]
    if run["mode"] == "leader_follower":
        cmd.extend(["--leader", LEADER_URL])

    run_command(cmd)


def main():
    parser = argparse.ArgumentParser(
        description=(
            "Deploy and run all hw10 load-test combinations sequentially. "
            "This script redeploys each mode, verifies cluster health, and runs the four required ratios one at a time."
        )
    )
    parser.add_argument("--workers", type=int, default=20, help="Worker threads per load test run.")
    parser.add_argument("--duration-s", type=int, default=60, help="Duration per load test run in seconds.")
    parser.add_argument(
        "--start-at",
        choices=[run["label"] for run in LEADER_FOLLOWER_RUNS],
        help="Optional mode label to resume from.",
    )
    args = parser.parse_args()

    start_found = args.start_at is None

    for run in LEADER_FOLLOWER_RUNS:
        if not start_found:
            if run["label"] == args.start_at:
                start_found = True
            else:
                continue

        print(f"\n=== {run['label']} ===")
        deploy_mode(run)
        time.sleep(5)
        verify_health(run["mode"], run["w"], run["r"])

        for ratio, ratio_label in RATIOS:
            print(f"\n--- ratio {ratio} ---")
            run_load_test(run, ratio, ratio_label, args.workers, args.duration_s)

    print("\nAll requested load tests completed.")


if __name__ == "__main__":
    main()
