# CS6650 — HW5: Product API with Terraform & Load Testing

## Overview

This repository contains the implementation for HW5: a Product API built with Go and the Gin framework, deployed to AWS ECS/Fargate using Terraform, and load tested with Locust.

---

## Repository Structure

| Location | Description |
|----------|-------------|
| `product-api/` | **Product API server code** (Go + Gin) |
| `product-api/main.go` | Server entry point and route registration |
| `product-api/handlers.go` | HTTP request handlers (GET, POST) |
| `product-api/models.go` | Data models (Product, ErrorResponse structs) |
| `product-api/store.go` | In-memory storage (thread-safe `sync.Map`) |
| `product-api/Dockerfile` | Docker configuration for the Product API |
| `product-api/locust/` | Locust load test files |
| `api.yaml` | OpenAPI specification for the e-commerce system |

**Infrastructure repo (separate):** [CS6650_2b_demo](https://github.com/RuidiH/CS6650_2b_demo) (forked)

| Location | Description |
|----------|-------------|
| `terraform/` | Terraform configs (ECR, ECS, networking, logging) |
| `terraform/main.tf` | Root module — wires ECR, ECS, network, logging modules |
| `terraform/variables.tf` | Configurable variables |
| `terraform/modules/ecr/` | Elastic Container Registry module |
| `terraform/modules/ecs/` | ECS Cluster, Task Definition, Service (Fargate) |
| `terraform/modules/network/` | VPC, Subnets, Security Groups |
| `terraform/modules/logging/` | CloudWatch log group |
| `src/Dockerfile` | Multi-stage Docker build used by Terraform |

---

## How to Run Locally

### Prerequisites
- Go 1.25+
- (Optional) Docker Desktop

### Run with Go

```bash
cd product-api
go run .
```

Server starts on `http://localhost:8080`.

### Run with Docker

```bash
cd product-api
docker build -t product-api .
docker run -p 8080:8080 product-api
```

---

## How to Deploy to AWS

### Prerequisites
- AWS CLI installed and configured
- Terraform installed
- Docker installed
- AWS account or Learner Lab (region: `us-west-2`)

### Step 1: Configure AWS Credentials

Copy credentials from your AWS Learner Lab into `~/.aws/credentials`:

```ini
[default]
aws_access_key_id=YOUR_ACCESS_KEY
aws_secret_access_key=YOUR_SECRET_KEY
aws_session_token=YOUR_SESSION_TOKEN
```

Set region in `~/.aws/config`:

```ini
[default]
region = us-west-2
```

Verify:

```bash
aws sts get-caller-identity
```

### Step 2: Clone the Infrastructure Repo

```bash
git clone https://github.com/<your-fork>/CS6650_2b_demo.git
```

### Step 3: Copy Product API into the Infrastructure Repo

```bash
cp product-api/main.go product-api/handlers.go product-api/models.go product-api/store.go CS6650_2b_demo/src/
cp product-api/go.mod product-api/go.sum CS6650_2b_demo/src/
```

### Step 4: Deploy with Terraform

```bash
cd CS6650_2b_demo/terraform
terraform init
terraform apply
```

Type `yes` when prompted. Terraform automatically:
1. Creates an ECR repository and pushes the Docker image
2. Creates an ECS cluster with a Fargate task
3. Sets up VPC, subnets, and security groups (port 8080 open)
4. Configures CloudWatch logging

### Step 5: Get the Public IP

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

### Step 6: Send Requests

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

Retrieve a product by ID.

| Parameter | Type | Constraints |
|-----------|------|-------------|
| `productId` (path) | integer | min: 1 |

### POST /products/{productId}/details

Add or update product details.

| Field | Type | Constraints |
|-------|------|-------------|
| `product_id` | int32 | min: 1, must match URL |
| `sku` | string | 1-100 characters |
| `manufacturer` | string | 1-200 characters |
| `category_id` | int32 | min: 1 |
| `weight` | int32 | min: 0 |
| `some_other_id` | int32 | min: 1 |

---

## Response Code Examples

### 204 — Product Created Successfully

```bash
curl -X POST http://localhost:8080/products/1/details \
  -H "Content-Type: application/json" \
  -d '{"product_id": 1, "sku": "ABC-123", "manufacturer": "Acme", "category_id": 1, "weight": 500, "some_other_id": 1}'
```

Response: _(empty body, 204 No Content)_

### 200 — Product Found

```bash
curl http://localhost:8080/products/1
```

```json
{"product_id":1,"sku":"ABC-123","manufacturer":"Acme","category_id":1,"weight":500,"some_other_id":1}
```

### 400 — Invalid ID Format

```bash
curl http://localhost:8080/products/abc
```

```json
{"error":"INVALID_INPUT","message":"invalid format of product_id"}
```

### 400 — Missing Required Fields

```bash
curl -X POST http://localhost:8080/products/1/details \
  -H "Content-Type: application/json" \
  -d '{"product_id": 1}'
```

```json
{"error":"INVALID_INPUT","message":"Key: 'Product.SKU' Error:Field validation for 'SKU' failed on the 'required' tag..."}
```

### 400 — URL ID Does Not Match Body

```bash
curl -X POST http://localhost:8080/products/1/details \
  -H "Content-Type: application/json" \
  -d '{"product_id": 999, "sku": "ABC", "manufacturer": "Acme", "category_id": 1, "weight": 500, "some_other_id": 1}'
```

```json
{"error":"INVALID_INPUT","message":"URL product ID does not match body product_id"}
```

### 404 — Product Not Found

```bash
curl http://localhost:8080/products/999
```

```json
{"error":"NOT_FOUND","message":"product not found"}
```

---

## Load Testing

Tests are in `product-api/locust/`. Install Locust with `pip install locust`.

### Run Tests

```bash
cd product-api/locust

# Against local server
locust -f locustfile.py --host=http://localhost:8080

# Against AWS server
locust -f locustfile_stress.py --host=http://<PUBLIC_IP>:8080
```

Open `http://localhost:8089` for the Locust web UI.

### Results Summary

| Experiment | Users | Wait | RPS | Observation |
|-----------|-------|------|-----|-------------|
| Local + HttpUser | 50 | 1-2s | ~33 | Bottleneck: wait_time |
| Local + FastHttpUser | 50 | 1-2s | ~33 | Bottleneck: wait_time |
| AWS + HttpUser | 50 | 1-2s | ~32 | +40ms network latency |
| AWS + FastHttpUser | 50 | 1-2s | ~32 | +40ms network latency |
| AWS + HttpUser (stress) | 200 | ~0s | ~2,122 | Bottleneck: server CPU |
| AWS + FastHttpUser (stress) | 200 | ~0s | ~2,076 | Bottleneck: server CPU |
| AWS + HttpUser (stress) | 5,000 | ~0s | ~3,768 | Client becomes bottleneck |
| AWS + FastHttpUser (stress) | 5,000 | ~0s | **~4,830** | **+28% — connection pooling wins** |

### Key Finding: HttpUser vs FastHttpUser

At 5,000 concurrent users with no wait time, **FastHttpUser achieved 28% higher throughput** (4,830 vs 3,768 RPS). FastHttpUser uses connection pooling and async I/O (gevent), which reuses TCP connections instead of opening a new one per request. At high concurrency, the connection management overhead in HttpUser becomes the bottleneck on the client machine, while FastHttpUser handles it efficiently.

