#!/bin/bash
export DATABASE_URL='postgres://postgres:postgres@172.31.52.19:5432/albumstore?sslmode=disable'
export AWS_REGION='us-west-2'
export S3_BUCKET='album-store-v1-deploy-stevi-1775872000'
export PORT='8080'
export WORKER_CONCURRENCY='50'
nohup ~/album-api > ~/album-api.log 2>&1 &
echo "API started with PID $!"
