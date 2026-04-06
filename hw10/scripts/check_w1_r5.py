#!/usr/bin/env python3

import argparse
import json
import threading
import time
import urllib.error
import urllib.parse
import urllib.request


def request_json(method, url, payload=None, timeout=2.0):
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
            body = json.loads(raw_body) if raw_body else None
            return {
                "status": response.getcode(),
                "body": body,
                "error": None,
                "started_at": started_at,
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
            "started_at": started_at,
            "elapsed_ms": round((time.perf_counter() - started_perf) * 1000, 2),
        }
    except Exception as err:  # noqa: BLE001
        return {
            "status": None,
            "body": None,
            "error": str(err),
            "started_at": started_at,
            "elapsed_ms": round((time.perf_counter() - started_perf) * 1000, 2),
        }


def run_parallel_reads(follower_url, key, timeout):
    query = urllib.parse.urlencode({"key": key})
    barrier = threading.Barrier(3)
    results = {}

    def issue_read(name, path):
        barrier.wait()
        results[name] = request_json("GET", f"{follower_url}/{path}?{query}", timeout=timeout)

    local_thread = threading.Thread(target=issue_read, args=("local_read", "local_read"), daemon=True)
    get_thread = threading.Thread(target=issue_read, args=("get", "get"), daemon=True)

    local_thread.start()
    get_thread.start()
    barrier.wait()

    local_thread.join()
    get_thread.join()
    return results


def is_fresh(result, expected_value, expected_version):
    body = result.get("body")
    return (
        result.get("status") == 200
        and isinstance(body, dict)
        and body.get("value") == expected_value
        and body.get("version") == expected_version
    )


def print_result(label, result):
    print(
        f"  {label}: status={result['status']} elapsed_ms={result['elapsed_ms']} "
        f"body={json.dumps(result['body']) if result['body'] is not None else 'null'} "
        f"error={result['error']}"
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


def main():
    parser = argparse.ArgumentParser(
        description=(
            "Check leader-follower W=1, R=5 behavior. "
            "After the leader acknowledges the write, the leader and follower public reads should be fresh, "
            "while raw follower local_read may still be stale."
        )
    )
    parser.add_argument("--leader", default="http://localhost:8080", help="Leader base URL.")
    parser.add_argument("--follower", default="http://localhost:8084", help="Follower base URL to probe.")
    parser.add_argument("--attempts", type=int, default=10, help="How many attempts to run.")
    parser.add_argument("--timeout", type=float, default=2.0, help="HTTP timeout in seconds.")
    parser.add_argument("--key-prefix", default="probe", help="Prefix for generated keys.")
    args = parser.parse_args()

    print(f"Leader:   {args.leader}")
    print(f"Follower: {args.follower}")
    print(f"Attempts: {args.attempts}")

    after_ack_consistent = 0
    observed_stale_local = 0

    for attempt in range(1, args.attempts + 1):
        key = f"{args.key_prefix}-{int(time.time_ns())}"
        value = f"value-{attempt}"

        print(f"\nAttempt {attempt}: key={key}")
        set_result = request_json(
            "POST",
            f"{args.leader}/set",
            payload={"key": key, "value": value},
            timeout=args.timeout,
        )
        print_result("set", set_result)

        if set_result["status"] != 201 or not isinstance(set_result["body"], dict):
            print("  write failed; stopping.")
            break

        expected_version = set_result["body"]["version"]
        read_results = run_parallel_reads(args.follower, key, args.timeout)
        local_result = read_results["local_read"]
        follower_get = read_results["get"]
        leader_get = request_json(
            "GET",
            f"{args.leader}/get?{urllib.parse.urlencode({'key': key})}",
            timeout=args.timeout,
        )

        print("  immediately after leader ack:")
        print(f"    follower local_read: {summarize_status(local_result, value, expected_version)}")
        print(f"    follower public get: {summarize_status(follower_get, value, expected_version)}")
        print("  after leader ack:")
        print(f"    leader get: {summarize_status(leader_get, value, expected_version)}")
        print(f"    follower get: {summarize_status(follower_get, value, expected_version)}")

        local_is_fresh = is_fresh(local_result, value, expected_version)
        leader_is_fresh = is_fresh(leader_get, value, expected_version)
        follower_get_is_fresh = is_fresh(follower_get, value, expected_version)
        stale_local = not local_is_fresh

        if leader_is_fresh and follower_get_is_fresh:
            after_ack_consistent += 1
        if stale_local:
            observed_stale_local += 1

        print(
            "  expected behavior observed: "
            f"{'yes' if leader_is_fresh and follower_get_is_fresh and stale_local else 'no'}"
        )

        time.sleep(0.1)

    print("\nSummary:")
    print(f"  leader and follower get returned 200 after ack: {after_ack_consistent}/{args.attempts}")
    print(f"  follower local_read was still stale immediately after ack: {observed_stale_local}/{args.attempts}")


if __name__ == "__main__":
    main()
