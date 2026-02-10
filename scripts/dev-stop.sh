#!/bin/bash
set -e

cd "$(dirname "$0")/.."

echo "Stopping dev databases..."
docker-compose -f docker-compose.dev.yml down
echo "Dev environment stopped."
