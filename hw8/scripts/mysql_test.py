#!/usr/bin/env python3

import json
import sys
import time
from datetime import datetime, timezone
from urllib import error, request


def iso_now():
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def timed_request(method, url, payload=None):
    data = None
    headers = {}
    if payload is not None:
      data = json.dumps(payload).encode("utf-8")
      headers["Content-Type"] = "application/json"

    req = request.Request(url, data=data, headers=headers, method=method)
    start = time.perf_counter()
    try:
        with request.urlopen(req, timeout=30) as resp:
            body = resp.read().decode("utf-8")
            elapsed_ms = (time.perf_counter() - start) * 1000
            return resp.status, body, elapsed_ms, True
    except error.HTTPError as exc:
        body = exc.read().decode("utf-8")
        elapsed_ms = (time.perf_counter() - start) * 1000
        return exc.code, body, elapsed_ms, False


def record(operation, elapsed_ms, success, status_code):
    return {
        "operation": operation,
        "response_time": round(elapsed_ms, 2),
        "success": success,
        "status_code": status_code,
        "timestamp": iso_now(),
    }


def main():
    if len(sys.argv) != 3:
        print("usage: mysql_test.py <base_url> <output_path>")
        sys.exit(1)

    base_url = sys.argv[1].rstrip("/")
    output_path = sys.argv[2]
    results = []

    status, _, _, success = timed_request(
        "POST",
        f"{base_url}/products/1/details",
        {
            "product_id": 1,
            "sku": "HW8-001",
            "manufacturer": "Acme",
            "category_id": 1,
            "weight": 500,
            "some_other_id": 1,
        },
    )
    if not success and status != 204:
        print("failed to seed product", file=sys.stderr)
        sys.exit(1)

    cart_ids = []

    for _ in range(50):
        status, body, elapsed_ms, success = timed_request(
            "POST",
            f"{base_url}/shopping-carts",
            {"customer_id": 42},
        )
        results.append(record("create_cart", elapsed_ms, success and status == 201, status))
        if status != 201:
            print("create cart failed", file=sys.stderr)
            sys.exit(1)

        payload = json.loads(body)
        cart_ids.append(payload["shopping_cart_id"])

    for cart_id in cart_ids:
        status, _, elapsed_ms, success = timed_request(
            "POST",
            f"{base_url}/shopping-carts/{cart_id}/items",
            {"product_id": 1, "quantity": 2},
        )
        results.append(record("add_items", elapsed_ms, success and status == 204, status))
        if status != 204:
            print("add items failed", file=sys.stderr)
            sys.exit(1)

    for cart_id in cart_ids:
        status, _, elapsed_ms, success = timed_request(
            "GET",
            f"{base_url}/shopping-carts/{cart_id}",
        )
        results.append(record("get_cart", elapsed_ms, success and status == 200, status))
        if status != 200:
            print("get cart failed", file=sys.stderr)
            sys.exit(1)

    with open(output_path, "w", encoding="utf-8") as fh:
        json.dump(results, fh, indent=2)

    print(f"wrote {len(results)} records to {output_path}")


if __name__ == "__main__":
    main()
