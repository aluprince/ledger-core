#!/bin/sh
set -e

echo "Running database migrations..."
migrate -path /migrations -database "$DATABASE_URL" up

echo "Starting API..."
exec /ledger-core
