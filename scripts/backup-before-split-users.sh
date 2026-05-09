#!/bin/bash
# backup-before-split-users.sh — pre-flight backup before the 00032 split-users
# migration. Runs against either dev or prod depending on args.
#
# Usage:
#   PGPASSWORD=<pwd> ./scripts/backup-before-split-users.sh dev
#   PGPASSWORD=<pwd> ./scripts/backup-before-split-users.sh prod
#
# Outputs (per env):
#   /home/carecomp/secrets/db_backups/<env>_pre_split_users_<ts>.dump   (pg_dump custom)
#   /home/carecomp/secrets/db_backups/joe_<env>_pre_split_users_<ts>/   (per-table CSVs + manifest)
#
# Verifies dump round-trips by listing tables. Aborts loudly on any failure.

set -euo pipefail

ENV="${1:?Usage: backup-before-split-users.sh <dev|prod>}"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
BACKUP_DIR="/home/carecomp/secrets/db_backups"

case "$ENV" in
    dev)
        HOST="localhost"
        DB="carecompanion"
        USER="carecompanion"
        ;;
    prod)
        HOST="carecompanion-db.cns7qg5iujxu.us-east-1.rds.amazonaws.com"
        DB="carecompanion"
        USER="carecompanion"
        ;;
    *)
        echo "FATAL: unknown env '$ENV' — must be 'dev' or 'prod'"
        exit 1
        ;;
esac

mkdir -p "$BACKUP_DIR"

DUMP_FILE="$BACKUP_DIR/${ENV}_pre_split_users_${TS}.dump"
JOE_DIR="$BACKUP_DIR/joe_${ENV}_pre_split_users_${TS}"

# Postgres 16 server requires pg_dump 16+ (Ubuntu 22.04 default is pg_dump 14
# which refuses to dump from a newer server). PGDG repo provides /usr/lib/postgresql/16/bin/pg_dump.
PG_DUMP=/usr/lib/postgresql/16/bin/pg_dump
if [ ! -x "$PG_DUMP" ]; then
    echo "FATAL: $PG_DUMP not found — install postgresql-client-16 from PGDG"
    exit 1
fi

echo "== full pg_dump from $HOST/$DB → $DUMP_FILE =="
"$PG_DUMP" --host="$HOST" --username="$USER" --dbname="$DB" \
    --format=custom --compress=9 --no-owner --no-acl \
    --file="$DUMP_FILE"

DUMP_SIZE=$(stat --printf='%s' "$DUMP_FILE")
echo "  dump size: $DUMP_SIZE bytes"

if [ "$DUMP_SIZE" -lt 100000 ]; then
    echo "FATAL: dump size suspiciously small (<100kB) — aborting"
    exit 1
fi

echo "== verify dump round-trips (pg_restore --list) =="
PG_RESTORE=/usr/lib/postgresql/16/bin/pg_restore
TABLE_COUNT=$("$PG_RESTORE" --list "$DUMP_FILE" | grep -c '^[0-9]\+; .* TABLE ' || true)
echo "  tables in dump: $TABLE_COUNT"

if [ "$TABLE_COUNT" -lt 30 ]; then
    echo "FATAL: dump has fewer than 30 tables — aborting"
    exit 1
fi

echo "== extract Joe Steinmetz family data (per-table CSV) → $JOE_DIR =="
"$(dirname "$0")/extract-joe-data.sh" "$HOST" "$DB" "$USER" "$JOE_DIR"

JOE_FILES=$(ls "$JOE_DIR"/*.csv 2>/dev/null | wc -l)
echo "  Joe CSV files: $JOE_FILES"

if [ "$JOE_FILES" -lt 30 ]; then
    echo "FATAL: Joe extract has fewer than 30 CSVs — aborting"
    exit 1
fi

echo
echo "Backup complete:"
echo "  full dump:  $DUMP_FILE  ($DUMP_SIZE bytes)"
echo "  joe extract: $JOE_DIR  ($JOE_FILES CSV files)"
echo
echo "Next: copy these to S3 for off-host durability before proceeding."
