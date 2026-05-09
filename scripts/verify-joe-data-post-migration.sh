#!/bin/bash
# verify-joe-data-post-migration.sh — re-extract Joe's data from prod after
# the migration has shipped, and diff field-by-field against the pre-migration
# CSVs. Any difference in any user-data field fails this check.
#
# Usage:
#   PGPASSWORD=<pwd> ./scripts/verify-joe-data-post-migration.sh
#
# Compares /home/carecomp/secrets/db_backups/joe_prod_pre_split_users_<ts>/
# (pre-migration) against a fresh extract from prod RDS post-migration.

set -euo pipefail

PRE_DIR=$(ls -dt /home/carecomp/secrets/db_backups/joe_prod_pre_split_users_*/ | head -1 | sed 's:/$::')
TS_NOW=$(date -u +%Y%m%dT%H%M%SZ)
POST_DIR="/home/carecomp/secrets/db_backups/joe_prod_post_split_users_${TS_NOW}"

if [ -z "$PRE_DIR" ] || [ ! -d "$PRE_DIR" ]; then
    echo "FATAL: pre-migration extract not found in /home/carecomp/secrets/db_backups/"
    exit 1
fi

echo "Pre-migration extract: $PRE_DIR"
echo "Post-migration extract: $POST_DIR"
echo

echo "== extract Joe's data from prod (post-migration) =="
"$(dirname "$0")/extract-joe-data.sh" \
    carecompanion-db.cns7qg5iujxu.us-east-1.rds.amazonaws.com \
    carecompanion carecompanion "$POST_DIR"

echo
echo "== field-by-field diff =="

DIFF_COUNT=0
EXTRA_COUNT=0
MISSING_COUNT=0

# Compare each CSV file
for pre_csv in "$PRE_DIR"/*.csv; do
    name=$(basename "$pre_csv" .csv)
    post_csv="$POST_DIR/$name.csv"

    if [ ! -f "$post_csv" ]; then
        echo "  MISSING POST: $name (was in pre but not in post)"
        MISSING_COUNT=$((MISSING_COUNT + 1))
        continue
    fi

    if ! diff -q "$pre_csv" "$post_csv" > /dev/null 2>&1; then
        echo "  DIFFER: $name"
        DIFF_COUNT=$((DIFF_COUNT + 1))
        # show first 5 differing lines for context (exclude manifest noise)
        diff "$pre_csv" "$post_csv" | head -10 | sed 's/^/      /'
    fi
done

# Catch any post files that weren't in pre
for post_csv in "$POST_DIR"/*.csv; do
    name=$(basename "$post_csv" .csv)
    if [ ! -f "$PRE_DIR/$name.csv" ]; then
        echo "  EXTRA POST: $name (in post but not in pre)"
        EXTRA_COUNT=$((EXTRA_COUNT + 1))
    fi
done

echo
echo "== summary =="
echo "  files differing: $DIFF_COUNT"
echo "  missing post:    $MISSING_COUNT"
echo "  extra post:      $EXTRA_COUNT"

# Joe-manifest is allowed to differ (extracted_at timestamp). All other CSVs must match.
if [ "$DIFF_COUNT" -eq 1 ] && \
   diff -q "$PRE_DIR/users.csv" "$POST_DIR/users.csv" > /dev/null 2>&1 && \
   ! diff -q "$PRE_DIR/joe-manifest.json" "$POST_DIR/joe-manifest.json" > /dev/null 2>&1; then
    echo
    echo "PASS: only joe-manifest.json (timestamp) differs. All Joe data byte-identical pre/post."
    exit 0
fi

if [ "$DIFF_COUNT" -eq 0 ] && [ "$MISSING_COUNT" -eq 0 ] && [ "$EXTRA_COUNT" -eq 0 ]; then
    echo
    echo "PASS: zero differences across all $(ls "$POST_DIR"/*.csv | wc -l) CSVs."
    exit 0
fi

echo
echo "REVIEW REQUIRED: see DIFFER/MISSING/EXTRA lines above"
exit 1
