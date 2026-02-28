"""
Experiment 0 — Extreme Spike Test (Broken Baseline).

Pattern:
  - 1 min baseline at 5 users
  - Spike to 100 users for 2 min
  - Drop to 5 users for 1 min (recovery)

Usage:
  locust -f locustfile_crash.py --host=http://<PUBLIC_IP>:8080
"""

from locust import task, constant, LoadTestShape
from locust.contrib.fasthttp import FastHttpUser
import random

SEARCH_TERMS = ["Electronics", "Books", "Home", "Garden", "Sports",
                "Alpha", "Beta", "Gamma", "Delta", "Epsilon",
                "Product", "Toys", "Food", "Clothing"]


class SearchUser(FastHttpUser):
    wait_time = constant(0)

    @task
    def search_products(self):
        term = random.choice(SEARCH_TERMS)
        self.client.get(f"/products/search?q={term}")


class ExtremeSpikeLoad(LoadTestShape):
    """
    Extreme spike shape:
      - Baseline:  5 users for 1 min
      - Spike:   300 users for 2 min
      - Recovery:  5 users for 1 min
    Total: 4 min
    """
    phases = [
        {"users": 5,   "duration": 60,  "spawn_rate": 5},    # 0:00 - 1:00  baseline
        {"users": 300, "duration": 120, "spawn_rate": 300},   # 1:00 - 3:00  SPIKE
        {"users": 5,   "duration": 60,  "spawn_rate": 300},   # 3:00 - 4:00  recovery
    ]

    def tick(self):
        run_time = self.get_run_time()
        elapsed = 0

        for phase in self.phases:
            elapsed += phase["duration"]
            if run_time < elapsed:
                return (phase["users"], phase["spawn_rate"])

        return None
