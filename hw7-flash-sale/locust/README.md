# Locust Runs

Install Locust:

```bash
pip install locust
```

Run from the project root:

```bash
cd /Users/stevi/Documents/web-service-gin/hw7-flash-sale
```

Async experiment:

```bash
ORDER_TEST_MODE=async locust -f locust/locustfile.py --host http://hw7-flash-sale-alb-475194094.us-west-2.elb.amazonaws.com
```

Sync comparison:

```bash
ORDER_TEST_MODE=sync locust -f locust/locustfile.py --host http://hw7-flash-sale-alb-475194094.us-west-2.elb.amazonaws.com
```

Locust UI:

```text
http://localhost:8089
```
