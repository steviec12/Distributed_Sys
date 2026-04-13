#!/bin/bash
curl -X POST http://chaosarena-alb-938452724.us-west-2.elb.amazonaws.com/submit \
  -H "Content-Type: application/json" \
  -d '{
    "email": "chen.yuang2@northeastern.edu",
    "nickname": "ifbbchen",
    "base_url": "http://35.88.18.209:8080",
    "contract": "v1-album-store"
  }'
echo
