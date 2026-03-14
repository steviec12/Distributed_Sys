import json
import os
import time
from itertools import count

from locust import FastHttpUser, between, task


ORDER_COUNTER = count(1)
TEST_MODE = os.getenv("ORDER_TEST_MODE", "async").strip().lower()


def build_order_payload():
    order_num = next(ORDER_COUNTER)
    return {
        "customer_id": order_num,
        "items": [
            {
                "product_id": 1,
                "quantity": 1,
            }
        ],
    }


class OrderUser(FastHttpUser):
    wait_time = between(0.1, 0.5)

    @task
    def submit_order(self):
        payload = build_order_payload()
        endpoint = "/orders/async"
        expected_code = 202
        expected_status = "pending"

        if TEST_MODE == "sync":
            endpoint = "/orders/sync"
            expected_code = 200
            expected_status = "completed"

        with self.client.post(
            endpoint,
            data=json.dumps(payload),
            headers={"Content-Type": "application/json"},
            catch_response=True,
            name=endpoint,
        ) as response:
            if response.status_code != expected_code:
                response.failure(
                    f"unexpected status code: {response.status_code} expected {expected_code}"
                )
                return

            try:
                body = response.json()
            except json.JSONDecodeError as exc:
                response.failure(f"invalid JSON response: {exc}")
                return

            if body.get("status") != expected_status:
                response.failure(
                    f"unexpected order status: {body.get('status')} expected {expected_status}"
                )
                return

            response.success()

    def on_start(self):
        # Give workers and AWS logs a tiny buffer before the user loop begins.
        time.sleep(0.25)
