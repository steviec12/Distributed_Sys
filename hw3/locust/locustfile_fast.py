from locust import task, between
from locust.contrib.fasthttp import FastHttpUser

class AlbumUser(FastHttpUser):
    wait_time = between(1, 2)  # Wait 1-2 seconds between tasks

    @task(3)  # Weight 3 (75% of requests)
    def get_albums(self):
        self.client.get("/albums")

    @task(1)  # Weight 1 (25% of requests)
    def post_album(self):
        self.client.post("/albums", json={
            "id": "99",
            "title": "Test Album",
            "artist": "Test Artist",
            "price": 19.99
        })

