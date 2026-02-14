"""
STRESS TEST with FastHttpUser - No wait time, high concurrency.
Compare with locustfile_stress.py to see if connection pooling matters at scale!

Usage:
  locust -f locustfile_stress_fast.py --host=http://16.144.83.110:8080
"""

from locust import task, between
from locust.contrib.fasthttp import FastHttpUser
import random


class StressUser(FastHttpUser):
    # Minimal wait — fire requests as fast as possible!
    wait_time = between(0, 0.1)

    product_ids = list(range(1, 101))

    def on_start(self):
        for pid in range(1, 11):
            self.client.post(
                f"/products/{pid}/details",
                json={
                    "product_id": pid,
                    "sku": f"SKU-{pid:04d}",
                    "manufacturer": f"Manufacturer-{pid}",
                    "category_id": (pid % 5) + 1,
                    "weight": random.randint(100, 5000),
                    "some_other_id": pid + 100,
                },
            )

    @task(5)
    def get_product(self):
        pid = random.randint(1, 10)
        self.client.get(f"/products/{pid}")

    @task(1)
    def post_product(self):
        pid = random.choice(self.product_ids)
        self.client.post(
            f"/products/{pid}/details",
            json={
                "product_id": pid,
                "sku": f"SKU-{pid:04d}",
                "manufacturer": f"LoadTest-Corp-{pid}",
                "category_id": random.randint(1, 10),
                "weight": random.randint(0, 10000),
                "some_other_id": random.randint(1, 999),
            },
        )

    @task(2)
    def get_nonexistent_product(self):
        pid = random.randint(500, 999)
        self.client.get(f"/products/{pid}")

