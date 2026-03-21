import json
import os
from datetime import datetime, timezone

import gevent
from gevent.lock import Semaphore
from locust import constant, events, task
from locust.contrib.fasthttp import FastHttpUser


EXPECTED_SESSIONS = int(os.getenv("EXPECTED_SESSIONS", "50"))
RESULTS_PATH = os.getenv("RESULTS_PATH", "results/mysql_test_results.json")

results = []
results_lock = Semaphore()
completion_lock = Semaphore()
completed_sessions = 0
product_seeded = False


def iso_from_timestamp(timestamp):
    return datetime.fromtimestamp(timestamp, timezone.utc).isoformat().replace("+00:00", "Z")


@events.test_start.add_listener
def seed_product(environment, **kwargs):
    global product_seeded

    if product_seeded:
        return

    if not environment.host:
        raise RuntimeError("Locust host is required")

    import urllib.request

    req = urllib.request.Request(
        f"{environment.host.rstrip('/')}/products/1/details",
        data=json.dumps(
            {
                "product_id": 1,
                "sku": "HW8-001",
                "manufacturer": "Acme",
                "category_id": 1,
                "weight": 500,
                "some_other_id": 1,
            }
        ).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )

    with urllib.request.urlopen(req, timeout=30) as response:
        if response.status != 204:
            raise RuntimeError("failed to seed product before Locust run")

    product_seeded = True


@events.request.add_listener
def record_request(request_type, name, response_time, response_length, response, context, exception, start_time, url, **kwargs):
    if name not in {"create_cart", "add_items", "get_cart"}:
        return

    status_code = getattr(response, "status_code", 0) if response is not None else 0
    success = exception is None and status_code < 400

    with results_lock:
        results.append(
            {
                "operation": name,
                "response_time": round(response_time, 2),
                "success": success,
                "status_code": status_code,
                "timestamp": iso_from_timestamp(start_time),
            }
        )


@events.test_stop.add_listener
def write_results(environment, **kwargs):
    os.makedirs(os.path.dirname(RESULTS_PATH), exist_ok=True)
    with open(RESULTS_PATH, "w", encoding="utf-8") as fh:
        json.dump(results, fh, indent=2)

    print(f"wrote {len(results)} records to {RESULTS_PATH}")


class ShoppingCartUser(FastHttpUser):
    wait_time = constant(0)

    def on_start(self):
        self.done = False
        self.cart_id = None

    @task
    def shopping_flow(self):
        global completed_sessions

        if self.done:
            gevent.sleep(1)
            return

        with self.client.post(
            "/shopping-carts",
            json={"customer_id": 42},
            name="create_cart",
            catch_response=True,
        ) as response:
            if response.status_code != 201:
                response.failure(f"expected 201, got {response.status_code}")
                self.environment.runner.quit()
                self.done = True
                return

            try:
                payload = response.json()
                self.cart_id = payload["shopping_cart_id"]
            except Exception as exc:
                response.failure(f"invalid cart response: {exc}")
                self.environment.runner.quit()
                self.done = True
                return

        with self.client.post(
            f"/shopping-carts/{self.cart_id}/items",
            json={"product_id": 1, "quantity": 2},
            name="add_items",
            catch_response=True,
        ) as response:
            if response.status_code != 204:
                response.failure(f"expected 204, got {response.status_code}")
                self.environment.runner.quit()
                self.done = True
                return

        with self.client.get(
            f"/shopping-carts/{self.cart_id}",
            name="get_cart",
            catch_response=True,
        ) as response:
            if response.status_code != 200:
                response.failure(f"expected 200, got {response.status_code}")
                self.environment.runner.quit()
                self.done = True
                return

        self.done = True

        with completion_lock:
            completed_sessions += 1
            if completed_sessions >= EXPECTED_SESSIONS:
                self.environment.runner.quit()
