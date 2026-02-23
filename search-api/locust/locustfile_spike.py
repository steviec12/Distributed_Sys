"""
Spike Test — Simulate a sudden traffic burst.

Pattern:
  - 2 min baseline at 5 users
  - Sudden spike to 50 users for 1 min
  - Drop back to 5 users for 1 min (recovery)

Usage:
  locust -f locustfile_spike.py --host=http://<PUBLIC_IP>:8080
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


class SpikeLoad(LoadTestShape):
    """
    Spike load shape:
      - Baseline phase:  5 users for 2 minutes
      - Spike phase:    50 users for 1 minute (sudden burst)
      - Recovery phase:  5 users for 1 minute (does it recover?)
    """
    phases = [
        {"users": 5,  "duration": 120, "spawn_rate": 5},   # 0:00 - 2:00  baseline
        {"users": 50, "duration": 60,  "spawn_rate": 50},  # 2:00 - 3:00  SPIKE
        {"users": 5,  "duration": 60,  "spawn_rate": 50},  # 3:00 - 4:00  recovery
    ]

    def tick(self):
        run_time = self.get_run_time()
        elapsed = 0

        for phase in self.phases:
            elapsed += phase["duration"]
            if run_time < elapsed:
                return (phase["users"], phase["spawn_rate"])

        # After all phases, stop
        return None

