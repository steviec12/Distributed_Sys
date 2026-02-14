"""
Load test for Product API using FastHttpUser.
FastHttpUser uses gevent + connection pooling for better performance.
Compare results with locustfile.py (HttpUser) to see the difference!

Usage:
  locust -f locustfile_fast.py --host=http://localhost:8080
  locust -f locustfile_fast.py --host=http://<AWS-PUBLIC-IP>:8080
"""

from locust import task, between
from locust.contrib.fasthttp import FastHttpUser
import random


class ProductUser(FastHttpUser):
    # Each simulated user waits 1-2 seconds between requests
    wait_time = between(1, 2)

    # Product IDs we'll use for testing
    product_ids = list(range(1, 101))  # 100 products

    def on_start(self):
        """Called when a simulated user starts. Seed some products first."""
        # Create 10 products so GET requests have something to find
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

    @task(5)  # Weight 5 — GET is 5x more common than POST (browsing > creating)
    def get_product(self):
        """Simulate a customer browsing a product page."""
        pid = random.randint(1, 10)  # Get one of the seeded products
        self.client.get(f"/products/{pid}")

    @task(1)  # Weight 1 — creating products is rare
    def post_product(self):
        """Simulate adding a new product."""
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

    @task(2)  # Weight 2 — some users will hit non-existent products (404s)
    def get_nonexistent_product(self):
        """Simulate a customer hitting a product that doesn't exist."""
        pid = random.randint(500, 999)  # High IDs unlikely to exist
        self.client.get(f"/products/{pid}")

