#!/bin/sh
set -e
chmod 600 /root/npm/data/jacques/jacques.env
chmod 755 /root/npm/data/jacques/jacques-bin
docker stop jacques || true
docker rm jacques || true
docker run -d --restart unless-stopped --name jacques --network npm_network -v /root/npm/data/jacques:/data --env-file /root/npm/data/jacques/jacques.env alpine:latest /data/jacques-bin
