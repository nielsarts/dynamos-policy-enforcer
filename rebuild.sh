#!/bin/bash
# Rebuild and restart the Docker Compose services
# This script stops containers, rebuilds images without cache, and starts them again

set -e

echo "Stopping containers..."
docker compose down

echo "Rebuilding images (no cache)..."
docker compose build --no-cache

echo "Starting containers..."
docker compose up -d

echo "Waiting for services to be ready..."
sleep 3

echo "Done! Services are running."
docker compose ps
