# Product API — HW5

A RESTful Product API for a simple e-commerce system, built with Go and the Gin framework. This API implements the Product endpoints defined in the OpenAPI specification (`api.yaml`), supporting product creation and retrieval with input validation and in-memory storage using Go's thread-safe `sync.Map`.

---

## Project Structure

```
web-service-gin/
├── product-api/
│   ├── main.go            — Server entry point and route registration
│   ├── handlers.go        — HTTP request handlers (GET, POST)
│   ├── models.go          — Data models (Product, ErrorResponse structs)
│   ├── store.go           — In-memory data storage (thread-safe sync.Map)
│   ├── Dockerfile         — Container configuration for the Product API
│   ├── go.mod             — Go module dependencies
│   ├── go.sum             — Dependency checksums
│   └── locust/
│       ├── locustfile.py          — Load test with HttpUser (standard)
│       ├── locustfile_fast.py     — Load test with FastHttpUser (connection pooling)
│       ├── locustfile_stress.py   — Stress test with HttpUser (no wait, high concurrency)
│       └── locustfile_stress_fast.py — Stress test with FastHttpUser (no wait, high concurrency)
├── api.yaml               — OpenAPI specification for the e-commerce system
└── HW_5_Instructions.md   — Assignment instructions

CS6650_2b_demo/            — Infrastructure repository (forked)
├── src/
│   └── Dockerfile         — Multi-stage Docker build for deployment
└── terraform/
    ├── provider.tf        — AWS provider configuration
    ├── variables.tf       — Configurable variables
    ├── main.tf            — Root module wiring ECR, ECS, networking, logging
    ├── outputs.tf         — Output values (cluster name, service name, etc.)
    └── modules/
        ├── ecr/           — Elastic Container Registry
        ├── ecs/           — ECS Cluster, Task Definition, Service (Fargate)
        ├── network/       — VPC, Subnets, Security Groups
        └── logging/       — CloudWatch log group
```

---

## How to Run Locally

### Prerequisites
- Go 1.25+ installed
- (Optional) Docker Desktop installed

### Run with Go

```bash
cd product-api
go run .
```

The server starts on `http://localhost:8080`.

### Run with Docker

```bash
cd product-api
docker build -t product-api .
docker run -p 8080:8080 product-api
```

---

## How to Deploy to AWS (Infrastructure)

### Prerequisites
- AWS CLI configured with credentials (`~/.aws/credentials`)
- Terraform installed
- Docker installed
- AWS Learner Lab (or AWS account) with `us-west-2` region

### Step 1: Configure AWS Credentials

From your AWS Learner Lab, copy the credentials into `~/.aws/credentials`:

```ini
[default]
aws_access_key_id=YOUR_ACCESS_KEY
aws_secret_access_key=YOUR_SECRET_KEY
aws_session_token=YOUR_SESSION_TOKEN
```

Also set the region in `~/.aws/config`:

```ini
[default]
region = us-west-2
```

Verify with:

```bash
aws sts get-caller-identity
```

### Step 2: Copy Product API Code into Infrastructure Repo

Copy your Product API source files into the `CS6650_2b_demo/src/` directory (replacing the demo server):

```bash
cp product-api/main.go product-api/handlers.go product-api/models.go product-api/store.go CS6650_2b_demo/src/
cp product-api/go.mod product-api/go.sum CS6650_2b_demo/src/
```

### Step 3: Deploy with Terraform

```bash
cd CS6650_2b_demo/terraform
terraform init
terraform apply
```

Type `yes` when prompted. Terraform will:
1. Create an ECR repository and push your Docker image
2. Create an ECS cluster with a Fargate task running your API
3. Set up networking (VPC, subnets, security groups with port 8080 open)
4. Configure CloudWatch logging

### Step 4: Get the Public IP

```bash
aws ec2 describe-network-interfaces \
  --network-interface-ids $(
    aws ecs describe-tasks \
      --cluster $(terraform output -raw ecs_cluster_name) \
      --tasks $(
        aws ecs list-tasks \
          --cluster $(terraform output -raw ecs_cluster_name) \
          --service-name $(terraform output -raw ecs_service_name) \
          --query 'taskArns[0]' --output text
      ) \
      --query "tasks[0].attachments[0].details[?name=='networkInterfaceId'].value" \
      --output text
  ) \
  --query 'NetworkInterfaces[0].Association.PublicIp' \
  --output text
```

### Step 5: Test the Deployed API

```bash
curl http://<PUBLIC_IP>:8080/products/1
```

### Cleanup

```bash
cd CS6650_2b_demo/terraform
terraform destroy
```

Type `yes` to remove all AWS resources.

---

## API Endpoints

### GET /products/{productId}

Retrieve a product by its ID.

- **Path Parameter:** `productId` (integer, minimum 1)
- **Success:** `200 OK` with product JSON
- **Errors:** `400 Bad Request`, `404 Not Found`

### POST /products/{productId}/details

Add or update product details.

- **Path Parameter:** `productId` (integer, minimum 1)
- **Request Body:** JSON with all required product fields
- **Success:** `204 No Content`
- **Errors:** `400 Bad Request`

**Required fields in request body:**

| Field | Type | Constraints |
|-------|------|-------------|
| `product_id` | int32 | min: 1, must match URL productId |
| `sku` | string | 1-100 characters |
| `manufacturer` | string | 1-200 characters |
| `category_id` | int32 | min: 1 |
| `weight` | int32 | min: 0 |
| `some_other_id` | int32 | min: 1 |

---

## Response Code Examples

### 200 — Product Found

```bash
# First create a product, then retrieve it
curl http://localhost:8080/products/1
```

Response:
```json
{"product_id":1,"sku":"ABC-123","manufacturer":"Acme","category_id":1,"weight":500,"some_other_id":1}
```

### 204 — Product Created/Updated Successfully

```bash
curl -X POST http://localhost:8080/products/1/details \
  -H "Content-Type: application/json" \
  -d '{"product_id": 1, "sku": "ABC-123", "manufacturer": "Acme", "category_id": 1, "weight": 500, "some_other_id": 1}'
```

Response: No body (204 No Content)

### 400 — Bad Request (Invalid ID Format)

```bash
curl http://localhost:8080/products/abc
```

Response:
```json
{"error":"INVALID_INPUT","message":"invalid format of product_id"}
```

### 400 — Bad Request (Missing Required Fields)

```bash
curl -X POST http://localhost:8080/products/1/details \
  -H "Content-Type: application/json" \
  -d '{"product_id": 1}'
```

Response:
```json
{"error":"INVALID_INPUT","message":"Key: 'Product.SKU' Error:Field validation for 'SKU' failed on the 'required' tag..."}
```

### 400 — Bad Request (ID Mismatch)

```bash
curl -X POST http://localhost:8080/products/1/details \
  -H "Content-Type: application/json" \
  -d '{"product_id": 999, "sku": "ABC", "manufacturer": "Acme", "category_id": 1, "weight": 500, "some_other_id": 1}'
```

Response:
```json
{"error":"INVALID_INPUT","message":"URL product ID does not match body product_id"}
```

### 404 — Product Not Found

```bash
curl http://localhost:8080/products/999
```

Response:
```json
{"error":"NOT_FOUND","message":"product not found"}
```

---

## Load Testing

Load tests are located in `product-api/locust/` and use [Locust](https://locust.io/).

### Prerequisites

```bash
pip install locust
```

### Running Tests

```bash
cd product-api/locust

# Standard test (HttpUser) — against local server
locust -f locustfile.py --host=http://localhost:8080

# Standard test (FastHttpUser) — against local server
locust -f locustfile_fast.py --host=http://localhost:8080

# Stress test (HttpUser) — against AWS
locust -f locustfile_stress.py --host=http://<PUBLIC_IP>:8080

# Stress test (FastHttpUser) — against AWS
locust -f locustfile_stress_fast.py --host=http://<PUBLIC_IP>:8080
```

Open the Locust web UI at `http://localhost:8089` to configure users and ramp-up rate.

### Load Test Results Summary

| Experiment | Users | Wait Time | RPS | Key Finding |
|-----------|-------|-----------|-----|-------------|
| Local + HttpUser | 50 | 1-2s | ~33 | Bottleneck: wait_time |
| Local + FastHttpUser | 50 | 1-2s | ~33 | Bottleneck: wait_time |
| AWS + HttpUser | 50 | 1-2s | ~32 | +40ms network latency per request |
| AWS + FastHttpUser | 50 | 1-2s | ~32 | +40ms network latency per request |
| AWS + HttpUser (stress) | 200 | ~0s | ~2,122 | Bottleneck: server CPU (0.25 vCPU) |
| AWS + FastHttpUser (stress) | 200 | ~0s | ~2,076 | Bottleneck: server CPU |
| AWS + HttpUser (stress) | 5,000 | ~0s | ~3,768 | Client becomes bottleneck |
| AWS + FastHttpUser (stress) | 5,000 | ~0s | **~4,830** | **28% higher RPS — connection pooling wins** |

### HttpUser vs FastHttpUser Analysis

At **low concurrency** (50-200 users), HttpUser and FastHttpUser perform nearly identically because:
- The `wait_time` or the server CPU is the bottleneck, not the HTTP client
- Both clients can easily manage a small number of connections

At **high concurrency** (5,000 users with no wait), **FastHttpUser outperforms HttpUser by ~28%** because:
- **HttpUser** (Python `requests`) opens a new TCP connection per request — at 5,000 users, the connection creation/teardown overhead consumes significant client CPU
- **FastHttpUser** (`geventhttpclient`) uses connection pooling and async I/O — reuses TCP connections and handles thousands of concurrent users more efficiently
- HttpUser's 95th percentile response times were higher and still climbing; FastHttpUser's were lower and stable
