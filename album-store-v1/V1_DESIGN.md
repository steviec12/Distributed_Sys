# Album Store V1 Design Record

This file records the architecture and design decisions agreed for the first implementation version of the distributed systems final project.

## Goal

Build a first version that prioritizes correctness, durability, and clear distributed-system behavior before later optimization.

## Agreed V1 Architecture

- Language/framework: Go + Gin
- API service: separate service
- Worker service: separate service
- Shared code: common package(s) for config, models, DB access, and shared types
- Metadata store: Postgres
- Object storage: S3
- Queue: SQS
- Public file access: direct accessible S3-backed URL until deletion

## Why This V1 Was Chosen

- Postgres is the source of truth for metadata and coordination.
- S3 stores file bytes, not system metadata.
- SQS carries async work for the worker.
- The worker handles post-accept finalization so the API stays focused on request handling.
- Redis is intentionally deferred as a possible optimization if V1 scoring shows a bottleneck.

## Core Principles

- Correctness comes before optimization in V1.
- Accepted uploads must not be lost.
- Per-album `seq` must be assigned synchronously in the request path.
- Duplicate photo uploads are treated as separate tasks.
- Same `album_id` on `PUT /albums/:album_id` means update the same album resource.
- Deleted photos should stay gone and must not be recreated by late worker activity.

## API Behavior Decisions

### Health

- `GET /health` returns exactly:

```json
{ "status": "ok" }
```

### Albums

- `PUT /albums/:album_id` is idempotent by `album_id`.
- If the album already exists, update it instead of creating a second record.
- If path `album_id` and body `album_id` differ, return `400`.
- `GET /albums/:album_id` returns `404` with `{ "error": "not found" }` if missing.
- `GET /albums` must return all albums with no hardcoded result limit.

### Photos

- Uploading a photo to a missing album returns `404`.
- Every accepted photo upload is a new task, even if the file contents are identical to a previous upload.
- Repeated `DELETE /albums/:album_id/photos/:photo_id` returns `204`.
- Deleted photos are treated as missing for reads, so `GET /albums/:album_id/photos/:photo_id` returns `404`.

## Upload Flow

### Request Path

The agreed V1 order for `POST /albums/:album_id/photos` is:

1. Generate `photo_id`
2. Store the upload durably in S3
3. Assign `seq` in Postgres
4. Write photo metadata with `status = processing`
5. Register async work
6. Return `202`

### Meaning Of `processing`

- `processing` does not mean the HTTP upload is still in flight.
- It means the upload was accepted and durably stored, but background finalization is not complete yet.

## Worker Behavior

The worker consumes jobs from SQS and then:

1. Loads the corresponding photo from Postgres
2. Checks whether the photo has been marked deleted
3. If deleted, stop processing and do not restore visible state
4. If not deleted, verify the S3 object exists
5. Build the public URL from the stored S3 key
6. Update the photo to `completed` and store the URL
7. If the object cannot be finalized correctly, mark the photo `failed`

The worker does not return client responses directly. It only updates system state that the API later reads.

## Delete Behavior

- Delete uses a tombstone/delete marker approach in Postgres.
- The photo is treated as deleted immediately at the metadata level.
- The S3 object must be removed before delete success is returned.
- After successful delete:
  - `GET /albums/:album_id/photos/:photo_id` returns `404`
  - the previous public file URL must no longer return `200`
- If the worker sees a deleted photo, it stops and must not recreate `completed` state.

## Postgres Schema For V1

### `albums`

Columns:

- `album_id`
- `title`
- `description`
- `owner`
- `next_seq`

Notes:

- `album_id` is the unique identity for an album.
- `next_seq` is the per-album sequence counter used for photo sequencing.
- `created_at` and `updated_at` were intentionally excluded from V1 because they are not needed for the assignment contract.

### `photos`

Columns:

- `photo_id`
- `album_id`
- `seq`
- `status`
- `s3_key`
- `url`
- `deleted`

Notes:

- `album_id` ties the photo to the album required by the API path.
- `seq` is required by the contract and scoped per album.
- `s3_key` is used internally by the worker and delete path.
- `url` is only populated once the photo reaches `completed`.
- `deleted` is an internal tombstone flag and is not returned as a public API status.

### `photo_jobs`

Columns:

- `job_id`
- `photo_id`
- `status`
- `attempts`

Notes:

- `attempts` is used for retry counting and poison-pill detection.
- `last_error` was intentionally excluded from V1.

## SQS Message Shape For V1

The first version keeps messages minimal:

```json
{ "photo_id": "..." }
```

The worker reads `photo_id` from SQS and loads the rest of the metadata from Postgres.

## File URL Strategy

- Completed photos use a direct S3-backed URL that is accessible without extra auth.
- This stays accessible until deletion.
- The system deletes the S3 object so the same URL stops returning `200`.

## Deferred Optimization

- Redis is not part of V1.
- Redis remains a possible optimization option if V1 scoring shows that Postgres-based `seq` assignment becomes a performance bottleneck.

## Project Layout Direction

The project will live in a separate folder:

- `/Users/stevi/Documents/web-service-gin/album-store-v1`

The implementation direction is:

- one assignment folder
- separate API service
- separate worker service
- shared internal/common package

## Open Implementation Details

These are implementation details still to be finalized while coding, not architecture reversals:

- exact SQL schema types and constraints
- exact `photo_jobs.status` values
- exact SQS publishing flow relative to any optional outbox handling
- exact S3 object key format
