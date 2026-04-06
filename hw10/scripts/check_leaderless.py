#!/usr/bin/env python3

import argparse
import json
import random
import threading
import time
import urllib.error
import urllib.parse
import urllib.request


def request_json(method, url, payload=None, timeout=3.0):
    data = None
    headers = {}
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"

    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    started_perf = time.perf_counter()

    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            raw_body = response.read().decode("utf-8")
            body = json.loads(raw_body) if raw_body else None
            return {
                "status": response.getcode(),
                "body": body,
                "error": None,
                "elapsed_ms": round((time.perf_counter() - started_perf) * 1000, 2),
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
            "elapsed_ms": round((time.perf_counter() - started_perf) * 1000, 2),
        }
    except Exception as err:  # noqa: BLE001
        return {
            "status": None,
            "body": None,
            "error": str(err),
            "elapsed_ms": round((time.perf_counter() - started_perf) * 1000, 2),
        }


def is_expected_200(result, expected_value, expected_version):
    body = result.get("body")
    return (
        result.get("status") == 200
        and isinstance(body, dict)
        and body.get("value") == expected_value
        and body.get("version") == expected_version
    )


def summarize_status(result, expected_value, expected_version):
    status = result.get("status")
    body = result.get("body")
    if status != 200:
        return str(status)
    if is_expected_200(result, expected_value, expected_version):
        return "200"
    if isinstance(body, dict):
        return f"200 (version={body.get('version')})"
    return "200"


def issue_set(coordinator_url, key, value, timeout, holder):
    holder["set"] = request_json(
        "POST",
        f"{coordinator_url}/set",
        payload={"key": key, "value": value},
        timeout=timeout,
    )


def capture_parallel_gets(targets, key, timeout):
    query = urllib.parse.urlencode({"key": key})
    results = {}
    threads = []

    def issue(node_label, node_url):
        results[node_label] = request_json("GET", f"{node_url}/get?{query}", timeout=timeout)

    for node_label, node_url in targets.items():
        thread = threading.Thread(target=issue, args=(node_label, node_url), daemon=True)
        threads.append(thread)
        thread.start()

    for thread in threads:
        thread.join()

    return results


def main():
    parser = argparse.ArgumentParser(
        description=(
            "Check the leaderless inconsistency window. "
            "Each attempt writes to a random coordinator, probes public GET on the other nodes during the write window, "
            "then verifies public GET on the coordinator and on another node after the coordinator acknowledges."
        )
    )
    parser.add_argument(
        "--nodes",
        default="http://localhost:8080,http://localhost:8081,http://localhost:8082,http://localhost:8083,http://localhost:8084",
        help="Comma-separated node base URLs in node1..node5 order.",
    )
    parser.add_argument("--attempts", type=int, default=10, help="How many attempts to run.")
    parser.add_argument("--timeout", type=float, default=3.0, help="HTTP timeout in seconds.")
    parser.add_argument("--early-delay-ms", type=float, default=25.0, help="Delay before the in-flight GET probe.")
    parser.add_argument("--key-prefix", default="leaderless", help="Prefix for generated keys.")
    parser.add_argument("--seed", type=int, default=10, help="Random seed for coordinator selection.")
    args = parser.parse_args()

    node_urls = [url.strip() for url in args.nodes.split(",") if url.strip()]
    if len(node_urls) != 5:
        raise SystemExit("--nodes must contain exactly 5 URLs in node1..node5 order")

    rng = random.Random(args.seed)

    print(f"Nodes:    {', '.join(node_urls)}")
    print(f"Attempts: {args.attempts}")
    print(f"Seed:     {args.seed}")

    attempts_with_early_stale = 0
    attempts_with_mixed_early_results = 0
    attempts_with_consistent_after_ack = 0

    for attempt in range(1, args.attempts + 1):
        key = f"{args.key_prefix}-{int(time.time_ns())}"
        value = f"value-{attempt}"
        coordinator_index = rng.randrange(len(node_urls))
        coordinator_label = f"node{coordinator_index + 1}"
        coordinator_url = node_urls[coordinator_index]

        other_indexes = [idx for idx in range(len(node_urls)) if idx != coordinator_index]
        after_ack_other_index = other_indexes[0]
        after_ack_other_label = f"node{after_ack_other_index + 1}"
        after_ack_other_url = node_urls[after_ack_other_index]

        inflight_targets = {f"node{idx + 1}": node_urls[idx] for idx in other_indexes}

        print(f"\nAttempt {attempt}: key={key}")
        print(f"  coordinator: {coordinator_label}")

        set_holder = {}
        set_thread = threading.Thread(
            target=issue_set,
            args=(coordinator_url, key, value, args.timeout, set_holder),
            daemon=True,
        )
        set_thread.start()

        time.sleep(args.early_delay_ms / 1000.0)
        inflight_gets = capture_parallel_gets(inflight_targets, key, args.timeout)

        set_thread.join()
        set_result = set_holder.get("set")
        if set_result is None:
            print("  write to coordinator: no result captured")
            continue

        print(f"  write to coordinator: {set_result['status']} ({set_result['elapsed_ms']} ms)")

        if set_result["status"] != 201 or not isinstance(set_result["body"], dict):
            print("  attempt failed before after-ack checks")
            continue

        expected_version = set_result["body"]["version"]

        print("  during write window (public get on other nodes):")
        early_stale_nodes = []
        early_fresh_nodes = []
        for node_label in sorted(inflight_gets):
            result = inflight_gets[node_label]
            status = summarize_status(result, value, expected_version)
            print(f"    {node_label} get: {status}")
            if is_expected_200(result, value, expected_version):
                early_fresh_nodes.append(node_label)
            else:
                early_stale_nodes.append(node_label)

        coordinator_get = request_json(
            "GET",
            f"{coordinator_url}/get?{urllib.parse.urlencode({'key': key})}",
            timeout=args.timeout,
        )
        other_get = request_json(
            "GET",
            f"{after_ack_other_url}/get?{urllib.parse.urlencode({'key': key})}",
            timeout=args.timeout,
        )

        print("  after coordinator ack:")
        print(f"    {coordinator_label} get: {summarize_status(coordinator_get, value, expected_version)}")
        print(f"    {after_ack_other_label} get: {summarize_status(other_get, value, expected_version)}")

        if early_stale_nodes:
            attempts_with_early_stale += 1
        if early_stale_nodes and early_fresh_nodes:
            attempts_with_mixed_early_results += 1

        after_ack_consistent = (
            is_expected_200(coordinator_get, value, expected_version)
            and is_expected_200(other_get, value, expected_version)
        )
        if after_ack_consistent:
            attempts_with_consistent_after_ack += 1

        print(
            f"  nodes with stale public get during write: "
            f"{', '.join(early_stale_nodes) if early_stale_nodes else 'none'}"
        )
        print(
            f"  nodes already updated during write: "
            f"{', '.join(early_fresh_nodes) if early_fresh_nodes else 'none'}"
        )
        print(f"  expected behavior observed: {'yes' if early_stale_nodes and after_ack_consistent else 'no'}")

        time.sleep(0.1)

    print("\nSummary:")
    print(f"  attempts with stale public get during the write window: {attempts_with_early_stale}/{args.attempts}")
    print(f"  attempts with mixed early results across other nodes: {attempts_with_mixed_early_results}/{args.attempts}")
    print(f"  attempts where coordinator get and another node get returned 200 after ack: {attempts_with_consistent_after_ack}/{args.attempts}")


if __name__ == "__main__":
    main()
