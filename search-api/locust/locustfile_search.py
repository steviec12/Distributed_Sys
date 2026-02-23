"""
Load test for Search API using FastHttpUser (no wait time).
Searches for common terms to stress the CPU on 0.25 vCPU Fargate.

Usage:
  Test 1 (Baseline):       locust -f locustfile_search.py --host=http://<PUBLIC_IP>:8080 --users 5 --spawn-rate 5 --run-time 2m --headless
  Test 2 (Breaking Point): locust -f locustfile_search.py --host=http://<PUBLIC_IP>:8080 --users 20 --spawn-rate 20 --run-time 3m --headless

  Or with Web UI:
  locust -f locustfile_search.py --host=http://<PUBLIC_IP>:8080
"""

from locust import task, constant
from locust.contrib.fasthttp import FastHttpUser
import random


# Common search terms that will match products in the generated data
SEARCH_TERMS = ["Electronics", "Books", "Home", "Garden", "Sports",
                "Alpha", "Beta", "Gamma", "Delta", "Epsilon",
                "Product", "Toys", "Food", "Clothing"]


class SearchUser(FastHttpUser):
    wait_time = constant(0)  # No wait time — fire requests as fast as possible

    @task
    def search_products(self):
        """Search for a random common term."""
        term = random.choice(SEARCH_TERMS)
        self.client.get(f"/products/search?q={term}")

