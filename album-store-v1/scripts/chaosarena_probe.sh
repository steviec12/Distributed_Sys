#!/usr/bin/env bash

set -uo pipefail

BASE="${BASE:-${1:-}}"
IMG="${IMG:-${2:-}}"
ALBUM_COUNT="${ALBUM_COUNT:-8}"
CONCURRENCY="${CONCURRENCY:-4}"
SAME_ALBUM_COUNT="${SAME_ALBUM_COUNT:-24}"
POLL_TIMEOUT_SECONDS="${POLL_TIMEOUT_SECONDS:-45}"

if [[ -z "$BASE" || -z "$IMG" ]]; then
  echo "usage: BASE=http://host:port IMG=/path/to/image $0"
  echo "   or: $0 http://host:port /path/to/image"
  exit 2
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required"
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required"
  exit 2
fi

if [[ ! -f "$IMG" ]]; then
  echo "image file not found: $IMG"
  exit 2
fi

TMP_ROOT="$(mktemp -d /tmp/chaosarena-probe.XXXXXX)"
trap 'rm -rf "$TMP_ROOT"' EXIT

failures=0

print_section() {
  echo
  echo "== $1 =="
}

note_failure() {
  failures=$((failures + 1))
  echo "FAIL: $1"
}

health_check() {
  print_section "Health"
  local body_file="$TMP_ROOT/health.body"
  local code
  code="$(curl -sS -o "$body_file" -w '%{http_code}' "$BASE/health")" || {
    note_failure "health request failed"
    return
  }

  echo "status=$code body=$(cat "$body_file")"
  if [[ "$code" != "200" ]]; then
    note_failure "health expected 200, got $code"
  fi
  if [[ "$(cat "$body_file")" != '{"status":"ok"}' ]]; then
    note_failure "health body is not the strict expected payload"
  fi
}

create_sparse_album() {
  local album_id="$1"
  local body_file="$TMP_ROOT/album-$album_id.body"
  local code

  code="$(curl -sS -o "$body_file" -w '%{http_code}' \
    -X PUT "$BASE/albums/$album_id" \
    -H 'Content-Type: application/json' \
    -d '{}')" || return 1

  echo "$code"
}

poll_photo() {
  local album_id="$1"
  local photo_id="$2"
  local started_at
  started_at="$(date +%s)"

  while true; do
    local body
    body="$(curl -sS "$BASE/albums/$album_id/photos/$photo_id")" || return 1
    local status
    status="$(echo "$body" | jq -r '.status // empty')"

    if [[ "$status" == "completed" ]]; then
      local url
      url="$(echo "$body" | jq -r '.url')"
      local url_code
      url_code="$(curl -sS -o /dev/null -w '%{http_code}' "$url")" || return 1
      if [[ "$url_code" != "200" ]]; then
        echo "completed but url returned $url_code: $url"
        return 1
      fi
      echo "completed $url"
      return 0
    fi

    if [[ "$status" == "failed" ]]; then
      echo "failed $body"
      return 1
    fi

    if (( "$(date +%s)" - started_at >= POLL_TIMEOUT_SECONDS )); then
      echo "timeout $body"
      return 1
    fi

    sleep 1
  done
}

upload_once() {
  local album_id="$1"
  local index="$2"
  local body_file="$TMP_ROOT/upload-$index.body"
  local code

  code="$(curl -sS -o "$body_file" -w '%{http_code}' \
    -X POST "$BASE/albums/$album_id/photos" \
    -F "photo=@$IMG")" || {
      printf '%s\t%s\trequest_failed\t-\n' "$index" "$album_id"
      return 0
    }

  local photo_id="-"
  if [[ "$code" == "202" ]]; then
    photo_id="$(jq -r '.photo_id // "-"' "$body_file")"
  fi

  printf '%s\t%s\t%s\t%s\n' "$index" "$album_id" "$code" "$photo_id"
}

burst_first_photo_many_albums() {
  print_section "Sparse Album Setup"
  local album_file="$TMP_ROOT/albums.tsv"
  : > "$album_file"

  local i
  for i in $(seq 1 "$ALBUM_COUNT"); do
    local album_id="probe-$(date +%s)-$i-$$"
    local code
    code="$(create_sparse_album "$album_id")" || code="request_failed"
    echo "album=$album_id status=$code body=$(cat "$TMP_ROOT/album-$album_id.body" 2>/dev/null || true)"
    printf '%s\t%s\n' "$i" "$album_id" >> "$album_file"
    if [[ "$code" != "200" && "$code" != "201" ]]; then
      note_failure "sparse album create failed for $album_id with $code"
    fi
  done

  print_section "First Photo Upload Burst Across Albums"
  local upload_file="$TMP_ROOT/first-uploads.tsv"
  : > "$upload_file"

  export BASE IMG TMP_ROOT
  export -f upload_once
  xargs -P "$CONCURRENCY" -n 2 bash -c 'upload_once "$2" "$1"' _ < "$album_file" | tee "$upload_file"

  local non202
  non202="$(awk -F '\t' '$3 != "202" {count++} END {print count+0}' "$upload_file")"
  echo "non_202_uploads=$non202 total=$ALBUM_COUNT"
  if [[ "$non202" != "0" ]]; then
    note_failure "one or more first-photo uploads returned non-202"
  fi

  print_section "Polling Accepted Photos"
  while IFS=$'\t' read -r index album_id code photo_id; do
    if [[ "$code" != "202" || "$photo_id" == "-" ]]; then
      continue
    fi
    echo "polling album=$album_id photo_id=$photo_id"
    if ! poll_photo "$album_id" "$photo_id"; then
      note_failure "photo did not complete cleanly for album $album_id"
    fi
  done < "$upload_file"
}

burst_same_album() {
  print_section "Same Album Concurrent Upload Burst"
  local album_id="probe-same-$(date +%s)-$$"
  local code
  code="$(create_sparse_album "$album_id")" || code="request_failed"
  echo "album=$album_id status=$code body=$(cat "$TMP_ROOT/album-$album_id.body" 2>/dev/null || true)"
  if [[ "$code" != "200" && "$code" != "201" ]]; then
    note_failure "same-album setup failed for $album_id"
    return
  fi

  local input_file="$TMP_ROOT/same-album-input.tsv"
  local output_file="$TMP_ROOT/same-album-uploads.tsv"
  : > "$input_file"
  : > "$output_file"

  local i
  for i in $(seq 1 "$SAME_ALBUM_COUNT"); do
    printf '%s\t%s\n' "$i" "$album_id" >> "$input_file"
  done

  export BASE IMG TMP_ROOT
  export -f upload_once
  xargs -P "$CONCURRENCY" -n 2 bash -c 'upload_once "$2" "$1"' _ < "$input_file" | tee "$output_file"

  local non202
  non202="$(awk -F '\t' '$3 != "202" {count++} END {print count+0}' "$output_file")"
  echo "non_202_same_album_uploads=$non202 total=$SAME_ALBUM_COUNT"
  if [[ "$non202" != "0" ]]; then
    note_failure "same-album concurrent uploads returned non-202"
  fi

  local seq_file="$TMP_ROOT/same-album-seqs.txt"
  : > "$seq_file"
  while IFS=$'\t' read -r _ _ upload_code photo_id; do
    if [[ "$upload_code" != "202" || "$photo_id" == "-" ]]; then
      continue
    fi
    local body
    body="$(curl -sS "$BASE/albums/$album_id/photos/$photo_id")" || continue
    echo "$body" | jq -r '.seq' >> "$seq_file"
  done < "$output_file"

  local seqs
  seqs="$(
    sort -n "$seq_file" | tr '\n' ' '
  )"
  echo "observed_initial_seqs=$seqs"

  local max_seq unique_count
  max_seq="$(sort -n "$seq_file" | tail -n 1)"
  unique_count="$(sort -n "$seq_file" | uniq | wc -l | tr -d ' ')"
  if [[ -n "$max_seq" ]]; then
    echo "same_album_unique_seq_count=$unique_count max_seq=$max_seq"
    if [[ "$max_seq" != "$unique_count" ]]; then
      note_failure "same-album seqs are not a dense 1..N progression"
    fi
  fi

  print_section "Polling Same Album Accepted Photos"
  while IFS=$'\t' read -r index same_album_id upload_code photo_id; do
    if [[ "$upload_code" != "202" || "$photo_id" == "-" ]]; then
      continue
    fi
    echo "polling same_album=$same_album_id photo_id=$photo_id index=$index"
    if ! poll_photo "$same_album_id" "$photo_id"; then
      note_failure "same-album photo did not complete cleanly for photo $photo_id"
    fi
  done < "$output_file"
}

health_check
burst_first_photo_many_albums
burst_same_album

print_section "Summary"
if [[ "$failures" -eq 0 ]]; then
  echo "probe completed without detected failures"
  exit 0
fi

echo "probe detected $failures failure(s)"
exit 1
