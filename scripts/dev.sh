#!/bin/bash
set -e

cd "$(dirname "$0")/.."

echo "=== MyCareCompanion Dev Environment ==="

# Start Postgres & Redis if not running
if ! docker ps --format '{{.Names}}' | grep -q carecompanion-dev-postgres; then
    echo "Starting Postgres and Redis..."
    docker-compose -f docker-compose.dev.yml up -d
    echo "Waiting for databases to be ready..."
    sleep 5

    # Verify health
    until docker exec carecompanion-dev-postgres pg_isready -U carecompanion > /dev/null 2>&1; do
        echo "  Waiting for Postgres..."
        sleep 2
    done
    echo "  Postgres ready."

    until docker exec carecompanion-dev-redis redis-cli ping > /dev/null 2>&1; do
        echo "  Waiting for Redis..."
        sleep 2
    done
    echo "  Redis ready."
else
    echo "Postgres and Redis already running."
fi

# Ensure Go and air are in PATH
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin

# Load dev environment
set -a
source .env.dev
set +a

# Run with hot-reload
echo ""
echo "Starting app with hot-reload on :${APP_PORT}..."
echo "Access at: http://98.88.131.147:${APP_PORT}"
echo "Press Ctrl+C to stop the app (databases keep running)."
echo ""
air
