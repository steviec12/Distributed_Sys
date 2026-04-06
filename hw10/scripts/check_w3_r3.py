#!/usr/bin/env python3

import argparse
import json
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
                "read_replicas": response.headers.get("X-Read-Replicas", ""),
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
            "read_replicas": err.headers.get("X-Read-Replicas", ""),
        }
    except Exception as err:  # noqa: BLE001
        return {
            "status": None,
            "body": None,
            "error": str(err),
            "elapsed_ms": round((time.perf_counter() - started_perf) * 1000, 2),
            "read_replicas": "",
        }


def is_fresh(result, expected_value, expected_version):
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
    if is_fresh(result, expected_value, expected_version):
        return "200"
    if isinstance(body, dict):
        return f"200 (version={body.get('version')})"
    return "200"


def issue_set(leader_url, key, value, timeout, holder):
    holder["set"] = request_json(
        "POST",
        f"{leader_url}/set",
        payload={"key": key, "value": value},
        timeout=timeout,
    )


def capture_local_reads_parallel(node_urls, key, timeout):
    query = urllib.parse.urlencode({"key": key})
    results = {}
    threads = []

    def issue(node_label, node_url):
        results[node_label] = request_json("GET", f"{node_url}/local_read?{query}", timeout=timeout)

    for index, node_url in enumerate(node_urls, start=1):
        label = f"node{index}"
        thread = threading.Thread(target=issue, args=(label, node_url), daemon=True)
        threads.append(thread)
        thread.start()

    for thread in threads:
        thread.join()

    return results


def capture_public_gets_parallel(node_urls, key, timeout):
    query = urllib.parse.urlencode({"key": key})
    results = {}
    threads = []

    def issue(node_label, node_url):
        results[node_label] = request_json("GET", f"{node_url}/get?{query}", timeout=timeout)

    for index, node_url in enumerate(node_urls, start=1):
        label = f"node{index}"
        thread = threading.Thread(target=issue, args=(label, node_url), daemon=True)
        threads.append(thread)
        thread.start()

    for thread in threads:
        thread.join()

    return results


def main():
    parser = argparse.ArgumentParser(
        description=(
            "Check leader-follower W=3, R=3. "
            "This captures a true in-flight local_read snapshot in parallel, then checks all nodes again after the leader ack."
        )
    )
    parser.add_argument(
        "--nodes",
        default="http://localhost:8080,http://localhost:8081,http://localhost:8082,http://localhost:8083,http://localhost:8084",
        help="Comma-separated node base URLs in node1..node5 order.",
    )
    parser.add_argument("--attempts", type=int, default=10, help="How many attempts to run.")
    parser.add_argument("--timeout", type=float, default=3.0, help="HTTP timeout in seconds.")
    parser.add_argument("--key-prefix", default="quorum", help="Prefix for generated keys.")
    parser.add_argument(
        "--probe-delay-ms",
        type=float,
        default=25.0,
        help="Delay after starting the write before capturing the in-flight local_read snapshot.",
    )
    parser.add_argument(
        "--quorum-write-threshold-ms",
        type=float,
        default=450.0,
        help="Minimum write latency to count as a quorum-like write.",
    )
    args = parser.parse_args()

    node_urls = [url.strip() for url in args.nodes.split(",") if url.strip()]
    if len(node_urls) != 5:
        raise SystemExit("--nodes must contain exactly 5 URLs in node1..node5 order")

    print(f"Nodes:    {', '.join(node_urls)}")
    print(f"Attempts: {args.attempts}")

    quorum_like_writes = 0
    attempts_with_inflight_stale = 0
    attempts_with_after_ack_stale = 0
    attempts_with_all_public_gets_ok = 0
    quorums_seen = set()

    for attempt in range(1, args.attempts + 1):
        key = f"{args.key_prefix}-{int(time.time_ns())}"
        value = f"value-{attempt}"

        print(f"\nAttempt {attempt}: key={key}")

        set_holder = {}
        set_thread = threading.Thread(
            target=issue_set,
            args=(node_urls[0], key, value, args.timeout, set_holder),
            daemon=True,
        )
        set_thread.start()

        time.sleep(args.probe_delay_ms / 1000.0)
        inflight_local_reads = capture_local_reads_parallel(node_urls, key, args.timeout)

        set_thread.join()
        set_result = set_holder.get("set")
        if set_result is None:
            print("  write to leader: no result captured")
            continue

        print(f"  write to leader: {set_result['status']} ({set_result['elapsed_ms']} ms)")

        if set_result["status"] != 201 or not isinstance(set_result["body"], dict):
            print("  attempt failed before after-ack checks")
            continue

        if set_result["elapsed_ms"] >= args.quorum_write_threshold_ms:
            quorum_like_writes += 1

        expected_version = set_result["body"]["version"]

        after_ack_local_reads = capture_local_reads_parallel(node_urls, key, args.timeout)
        after_ack_public_gets = capture_public_gets_parallel(node_urls, key, args.timeout)

        inflight_stale_nodes = []
        print("  during leader write:")
        for index in range(1, 6):
            label = f"node{index}"
            local_result = inflight_local_reads[label]
            local_status = summarize_status(local_result, value, expected_version)
            print(f"    {label} local_read: {local_status}")
            if local_status != "200":
                inflight_stale_nodes.append(label)

        after_ack_stale_nodes = []
        after_ack_public_ok = True
        print("  after leader ack:")
        for index in range(1, 6):
            label = f"node{index}"
            local_result = after_ack_local_reads[label]
            public_result = after_ack_public_gets[label]
            local_status = summarize_status(local_result, value, expected_version)
            public_status = summarize_status(public_result, value, expected_version)
            quorum_used = public_result.get("read_replicas") or "-"
            print(f"    {label} local_read: {local_status}")
            print(f"    {label} public get: {public_status} | quorum: {quorum_used}")
            if local_status != "200":
                after_ack_stale_nodes.append(label)
            if not is_fresh(public_result, value, expected_version):
                after_ack_public_ok = False
            if quorum_used and quorum_used != "-":
                quorums_seen.add(quorum_used)

        if inflight_stale_nodes:
            attempts_with_inflight_stale += 1
        if after_ack_stale_nodes:
            attempts_with_after_ack_stale += 1
        if after_ack_public_ok:
            attempts_with_all_public_gets_ok += 1

        print(
            f"  nodes still stale during leader write: "
            f"{', '.join(inflight_stale_nodes) if inflight_stale_nodes else 'none'}"
        )
        print(
            f"  nodes still stale after leader ack: "
            f"{', '.join(after_ack_stale_nodes) if after_ack_stale_nodes else 'none'}"
        )
        print(
            f"  write looked like a quorum write: "
            f"{'yes' if set_result['elapsed_ms'] >= args.quorum_write_threshold_ms else 'no'}"
        )
        print(f"  all public gets returned 200 after ack: {'yes' if after_ack_public_ok else 'no'}")

        time.sleep(0.1)

    print("\nSummary:")
    print(f"  writes that looked like quorum writes: {quorum_like_writes}/{args.attempts}")
    print(f"  attempts where some node was stale during the write: {attempts_with_inflight_stale}/{args.attempts}")
    print(f"  attempts where some node was still stale after ack: {attempts_with_after_ack_stale}/{args.attempts}")
    print(f"  attempts where all 5 public gets returned 200 after ack: {attempts_with_all_public_gets_ok}/{args.attempts}")
    print(f"  distinct quorums observed: {len(quorums_seen)}")
    for quorum in sorted(quorums_seen):
        print(f"    {quorum}")


if __name__ == "__main__":
    main()
