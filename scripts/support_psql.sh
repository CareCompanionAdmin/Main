#!/usr/bin/env bash
# Wrapper for querying the shared support DB (prod RDS support tables) from dev.
# Reads SUPPORT_DB_DSN from .env + the password from the secrets file, then runs
# psql with any args passed through. Lets support-ticket queries be a single,
# allowlistable command instead of a compound block that re-derives the DSN.
#
# Usage:
#   scripts/support_psql.sh -c "SELECT ..."
#   scripts/support_psql.sh -f scripts/some.sql
set -euo pipefail
cd /home/carecomp/carecompanion

DSN=$(grep '^SUPPORT_DB_DSN=' .env | sed 's/^SUPPORT_DB_DSN=//')
HOST=$(echo "$DSN" | grep -oP 'host=\K[^ ]+')
PORT=$(echo "$DSN" | grep -oP 'port=\K[^ ]+')
USER=$(echo "$DSN" | grep -oP 'user=\K[^ ]+')
DB=$(echo "$DSN" | grep -oP 'dbname=\K[^ ]+')
export PGPASSWORD=$(cat /home/carecomp/secrets/prod_support_db_pass.txt)

exec psql "host=$HOST port=$PORT user=$USER dbname=$DB sslmode=require" -P pager=off "$@"
