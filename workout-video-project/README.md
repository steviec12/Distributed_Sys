# Workout Video Project

This directory contains the first working prototype for the final project:

- Go API for async job submission and status lookup
- Redis queue and job state storage
- Python worker for video inspection
- Docker Compose for running the stack locally

## Milestone 1 Scope

The current prototype focuses on the distributed backend rather than full AI analysis.

- `POST /jobs` accepts a workout video upload and returns a `job_id`
- `GET /jobs/:id` returns the current job record
- Redis stores one hash per job and one pending queue
- The worker uses `ffprobe` to extract `duration_seconds`
- The worker simulates a heavier analysis stage using the extracted duration

## Project Layout

- `api/` Go API service
- `worker/` Python worker service
- `uploads/` shared upload directory for submitted videos
- `docker-compose.yml` local stack definition

## Run With Docker Compose

```bash
cd workout-video-project
docker compose up --build
```

The API will be available at `http://localhost:8080`.

## Example Requests

Submit a video:

```bash
curl -X POST http://localhost:8080/jobs \
  -F "video=@/absolute/path/to/workout.mp4"
```

Check job status:

```bash
curl http://localhost:8080/jobs/<job_id>
```

## Redis Layout

- `job:{job_id}`: Redis hash for a single job
- `queue:pending`: Redis list containing queued job IDs
- `stats:jobs_submitted`: counter
- `stats:jobs_completed`: counter
- `stats:jobs_failed`: counter

