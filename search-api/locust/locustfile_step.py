"""
Step Load Test — Find the exact breaking point.

Ramps users in steps: 5 → 10 → 20 → 30 → 40
Holds each level for 60 seconds.

Usage:
  locust -f locustfile_step.py --host=http://<PUBLIC_IP>:8080
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


class StepLoad(LoadTestShape):
    """
    Step load shape:
      - Each step holds for step_duration seconds
      - Users increase at each step
      - spawn_rate controls how fast users ramp within each step
    """
    steps = [
        {"users": 5,  "duration": 60},   # 0:00 - 1:00
        {"users": 10, "duration": 60},   # 1:00 - 2:00
        {"users": 20, "duration": 60},   # 2:00 - 3:00
        {"users": 30, "duration": 60},   # 3:00 - 4:00
        {"users": 40, "duration": 60},   # 4:00 - 5:00
    ]
    spawn_rate = 10  # users per second when ramping

    def tick(self):
        run_time = self.get_run_time()
        elapsed = 0

        for step in self.steps:
            elapsed += step["duration"]
            if run_time < elapsed:
                return (step["users"], self.spawn_rate)

        # After all steps, stop the test
        return None

