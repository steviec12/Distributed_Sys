# Album Store V1 Code Map

This file is a short guide to the current implementation, not a replacement for the contract.

## Services

- [cmd/api/main.go](/Users/stevi/Documents/web-service-gin/album-store-v1/cmd/api/main.go): starts the HTTP API
- [cmd/worker/main.go](/Users/stevi/Documents/web-service-gin/album-store-v1/cmd/worker/main.go): runs the async background worker

## Main Packages

- [internal/api](/Users/stevi/Documents/web-service-gin/album-store-v1/internal/api): HTTP handlers and request validation
- [internal/store](/Users/stevi/Documents/web-service-gin/album-store-v1/internal/store): Postgres access and transactions
- [internal/storage](/Users/stevi/Documents/web-service-gin/album-store-v1/internal/storage): S3 object operations and public URL building
- [internal/queue](/Users/stevi/Documents/web-service-gin/album-store-v1/internal/queue): SQS publish/receive/delete operations
- [internal/worker](/Users/stevi/Documents/web-service-gin/album-store-v1/internal/worker): outbox publishing and queue-consumer finalization

## Important Flows

### Album metadata

- `PUT /albums/:album_id` calls the album store upsert in [internal/store/albums.go](/Users/stevi/Documents/web-service-gin/album-store-v1/internal/store/albums.go)
- Postgres handles the race with `ON CONFLICT`, not Go code

### Photo upload

- `POST /albums/:album_id/photos` lives in [internal/api/photos.go](/Users/stevi/Documents/web-service-gin/album-store-v1/internal/api/photos.go)
- The API uploads the file to S3 first
- Then Postgres creates:
  - a `photos` row with `status = processing`
  - a `photo_jobs` row with `status = pending`
- The API returns `202` after the DB transaction succeeds

### Outbox publishing

- Pending rows in `photo_jobs` are the durable outbox
- [internal/worker/service.go](/Users/stevi/Documents/web-service-gin/album-store-v1/internal/worker/service.go) publishes pending jobs to SQS
- Successful publish changes the job to `published`

### Photo completion

- The worker consumes SQS messages
- It loads the photo by `photo_id`
- If the S3 object exists, it marks the photo `completed` and stores the public URL
- If the object is missing, it marks the photo `failed`
- If the photo was deleted or missing, the worker treats the message as terminal and does not recreate photo state

## Where To Change Things Later

- Album API behavior: [internal/api/albums.go](/Users/stevi/Documents/web-service-gin/album-store-v1/internal/api/albums.go)
- Photo API behavior: [internal/api/photos.go](/Users/stevi/Documents/web-service-gin/album-store-v1/internal/api/photos.go)
- Postgres queries and transactions: [internal/store](/Users/stevi/Documents/web-service-gin/album-store-v1/internal/store)
- Queue behavior and SQS details: [internal/queue/sqs.go](/Users/stevi/Documents/web-service-gin/album-store-v1/internal/queue/sqs.go)
- Worker orchestration: [internal/worker/service.go](/Users/stevi/Documents/web-service-gin/album-store-v1/internal/worker/service.go)
