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


def is_fresh(result, expected_value, expected_version):
    body = result.get("body")
    return (
        result.get("status") == 200
        and isinstance(body, dict)
        and body.get("value") == expected_value
        and body.get("version") == expected_version
    )


def summarize_state(result, expected_value, expected_version):
    if is_fresh(result, expected_value, expected_version):
        return "200"

    status = result.get("status")
    body = result.get("body")
    if status != 200:
        return str(status)
    if isinstance(body, dict):
        return f"200(v={body.get('version')}, stale)"
    return "200(stale)"


def issue_set(leader_url, key, value, timeout, holder, done_event):
    try:
        holder["set"] = request_json(
            "POST",
            f"{leader_url}/set",
            payload={"key": key, "value": value},
            timeout=timeout,
        )
    finally:
        done_event.set()


def track_follower_progress(follower_urls, key, expected_value_getter, expected_version_getter, timeout, poll_interval_ms, done_event):
    query = urllib.parse.urlencode({"key": key})
    progress = {
        label: {"first_stale": None, "first_fresh": None}
        for label in follower_urls
    }

    while not done_event.is_set():
        expected_value = expected_value_getter()
        expected_version = expected_version_getter()

        for label, follower_url in follower_urls.items():
            result = request_json("GET", f"{follower_url}/local_read?{query}", timeout=timeout)
            if expected_version is None:
                state = str(result.get("status"))
            else:
                state = summarize_state(result, expected_value, expected_version)

            if state != "200" and progress[label]["first_stale"] is None:
                progress[label]["first_stale"] = state
            if state == "200" and progress[label]["first_fresh"] is None:
                progress[label]["first_fresh"] = state

        time.sleep(poll_interval_ms / 1000.0)

    return progress


def main():
    parser = argparse.ArgumentParser(
        description=(
            "Check leader-follower W=5, R=1 progression. "
            "This shows follower local_read moving from stale to fresh during the leader write, "
            "then verifies leader and follower public reads are fresh after the leader acknowledges."
        )
    )
    parser.add_argument("--leader", default="http://localhost:8080", help="Leader base URL.")
    parser.add_argument(
        "--followers",
        default="http://localhost:8081,http://localhost:8082,http://localhost:8083,http://localhost:8084",
        help="Comma-separated follower base URLs.",
    )
    parser.add_argument(
        "--post-ack-follower",
        default="http://localhost:8081",
        help="Follower base URL to use for the required post-ack public get check.",
    )
    parser.add_argument("--attempts", type=int, default=5, help="How many attempts to run.")
    parser.add_argument("--timeout", type=float, default=3.0, help="HTTP timeout in seconds.")
    parser.add_argument("--poll-interval-ms", type=float, default=40.0, help="Follower local_read poll interval.")
    parser.add_argument("--key-prefix", default="lf-w5r1", help="Prefix for generated keys.")
    args = parser.parse_args()

    follower_url_list = [url.strip() for url in args.followers.split(",") if url.strip()]
    follower_urls = {f"node{index}": url for index, url in enumerate(follower_url_list, start=2)}

    print(f"Leader:            {args.leader}")
    print(f"Followers:         {', '.join(follower_urls.values())}")
    print(f"Post-ack follower: {args.post_ack_follower}")
    print(f"Attempts:          {args.attempts}")

    after_ack_consistent = 0
    all_followers_reached_fresh_before_ack = 0

    for attempt in range(1, args.attempts + 1):
        key = f"{args.key_prefix}-{int(time.time_ns())}"
        value = f"value-{attempt}"
        version_box = {"value": None}

        print(f"\nAttempt {attempt}: key={key}")
        set_holder = {}
        done_event = threading.Event()
        set_thread = threading.Thread(
            target=issue_set,
            args=(args.leader, key, value, args.timeout, set_holder, done_event),
            daemon=True,
        )
        set_thread.start()

        progress = track_follower_progress(
            follower_urls,
            key,
            expected_value_getter=lambda: value,
            expected_version_getter=lambda: version_box["value"],
            timeout=args.timeout,
            poll_interval_ms=args.poll_interval_ms,
            done_event=done_event,
        )

        set_result = set_holder.get("set")
        set_thread.join()
        if set_result is None:
            print("  write to leader: no result captured")
            continue

        if set_result["status"] == 201 and isinstance(set_result["body"], dict):
            version_box["value"] = set_result["body"]["version"]

        print(f"  write to leader: {set_result['status']} ({set_result['elapsed_ms']} ms)")

        if set_result["status"] != 201 or not isinstance(set_result["body"], dict):
            print("  attempt failed before post-ack checks")
            continue

        expected_version = set_result["body"]["version"]

        print("  during leader write:")
        followers_reached_fresh = True
        for label in sorted(follower_urls):
            follower_url = follower_urls[label]
            final_local = request_json(
                "GET",
                f"{follower_url}/local_read?{urllib.parse.urlencode({'key': key})}",
                timeout=args.timeout,
            )
            first_stale = progress[label]["first_stale"]
            final_state = summarize_state(final_local, value, expected_version)
            if final_state != "200":
                followers_reached_fresh = False

            if first_stale is None:
                print(f"    {label} local_read: started fresh -> {final_state}")
            else:
                print(f"    {label} local_read: {first_stale} -> {final_state}")

        if followers_reached_fresh:
            all_followers_reached_fresh_before_ack += 1

        leader_get = request_json(
            "GET",
            f"{args.leader}/get?{urllib.parse.urlencode({'key': key})}",
            timeout=args.timeout,
        )
        follower_get = request_json(
            "GET",
            f"{args.post_ack_follower}/get?{urllib.parse.urlencode({'key': key})}",
            timeout=args.timeout,
        )

        leader_fresh = is_fresh(leader_get, value, expected_version)
        follower_fresh = is_fresh(follower_get, value, expected_version)
        if leader_fresh and follower_fresh and followers_reached_fresh:
            after_ack_consistent += 1

        print("  after leader ack:")
        print(f"    leader get: {'200' if leader_fresh else summarize_state(leader_get, value, expected_version)}")
        print(f"    follower get: {'200' if follower_fresh else summarize_state(follower_get, value, expected_version)}")
        print(f"    all follower local_read: {'200' if followers_reached_fresh else 'not all fresh'}")

        print(
            "  verdict: "
            f"leader_consistent_after_ack={leader_fresh} "
            f"follower_consistent_after_ack={follower_fresh} "
            f"all_followers_fresh_after_ack={followers_reached_fresh}"
        )

        time.sleep(0.1)

    print("\nSummary:")
    print(f"  post-ack leader/follower consistency: {after_ack_consistent}/{args.attempts}")
    print(f"  all followers reached 200 by leader ack: {all_followers_reached_fresh_before_ack}/{args.attempts}")


if __name__ == "__main__":
    main()
