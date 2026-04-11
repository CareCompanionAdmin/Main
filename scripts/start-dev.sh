#!/bin/bash
# Start script for systemd: ensures Docker containers are healthy before launching the app.
set -e

cd /home/carecomp/carecompanion

# Ensure dev containers are running
if ! docker ps --format '{{.Names}}' | grep -q carecompanion-dev-postgres; then
    echo "Starting Postgres and Redis containers..."
    docker-compose -f docker-compose.dev.yml up -d
fi

# Wait for Postgres to accept connections (up to 60s)
echo "Waiting for Postgres..."
for i in $(seq 1 30); do
    if docker exec carecompanion-dev-postgres pg_isready -U carecompanion > /dev/null 2>&1; then
        echo "Postgres ready."
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "ERROR: Postgres not ready after 60s" >&2
        exit 1
    fi
    sleep 2
done

# Wait for Redis to accept connections (up to 30s)
echo "Waiting for Redis..."
for i in $(seq 1 15); do
    if docker exec carecompanion-dev-redis redis-cli ping > /dev/null 2>&1; then
        echo "Redis ready."
        break
    fi
    if [ "$i" -eq 15 ]; then
        echo "ERROR: Redis not ready after 30s" >&2
        exit 1
    fi
    sleep 2
done

echo "Dependencies ready. Starting MyCareCompanion..."
exec /home/carecomp/carecompanion/bin/carecompanion
