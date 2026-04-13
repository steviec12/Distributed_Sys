-- Albums are the top-level resource and also hold the per-album photo counter.
CREATE TABLE albums (
    album_id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    owner TEXT NOT NULL,
    next_seq INTEGER NOT NULL DEFAULT 1 CHECK (next_seq >= 1)
);

-- Photos track the public lifecycle plus the S3 object reference used internally.
CREATE TABLE photos (
    photo_id TEXT PRIMARY KEY,
    album_id TEXT NOT NULL REFERENCES albums(album_id) ON DELETE RESTRICT,
    seq INTEGER NOT NULL CHECK (seq >= 1),
    status TEXT NOT NULL CHECK (status IN ('processing', 'completed', 'failed')),
    s3_key TEXT NOT NULL,
    temp_path TEXT,
    content_type TEXT,
    url TEXT,
    deleted BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (album_id, seq)
);

CREATE INDEX photos_album_id_idx ON photos(album_id);
CREATE INDEX photos_deleted_idx ON photos(deleted);

-- Photo jobs are the durable outbox rows that bridge Postgres and SQS.
CREATE TABLE photo_jobs (
    job_id TEXT PRIMARY KEY,
    photo_id TEXT NOT NULL REFERENCES photos(photo_id) ON DELETE RESTRICT,
    status TEXT NOT NULL CHECK (status IN ('pending', 'published', 'processing', 'completed', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0)
);

CREATE INDEX photo_jobs_photo_id_idx ON photo_jobs(photo_id);
CREATE INDEX photo_jobs_status_idx ON photo_jobs(status);
