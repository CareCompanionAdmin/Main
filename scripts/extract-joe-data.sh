#!/bin/bash
# extract-joe-data.sh — pull EVERY row of EVERY table that holds Joe Steinmetz's
# data (and his children's data, especially Matty's), partitioned per-table for
# field-by-field comparison post-migration.
#
# Usage:
#   PGPASSWORD=<pwd> ./scripts/extract-joe-data.sh <host> <db> <user> <output_dir>
#
# Output: <output_dir>/<table>.csv — one CSV per table, all rows that reference
# any user_id/family_id/child_id we identified as Joe's. Plus joe-manifest.json
# with the IDs we resolved at extraction time so post-migration verification
# can use the same scope.

set -euo pipefail

HOST="${1:-localhost}"
DB="${2:-carecompanion}"
USER="${3:-carecompanion}"
OUTDIR="${4:?Usage: extract-joe-data.sh <host> <db> <user> <output_dir>}"

mkdir -p "$OUTDIR"

PSQL="psql -h $HOST -U $USER -d $DB -X --quiet --tuples-only --csv"

echo "== resolving Joe's identity scope on $HOST/$DB =="

# Joe's user IDs (any row whose last_name='Steinmetz' OR email matches the known patterns)
JOE_USER_IDS=$($PSQL -c "
    SELECT id FROM users
    WHERE last_name ILIKE 'steinmetz'
       OR email ILIKE '%gooseneckmedia%'
       OR email ILIKE '%workmaninsurancegroup%';
" | tr -d ' ' | grep -v '^$' | paste -sd, -)
echo "  user_ids: $JOE_USER_IDS"

# Family IDs that any Joe is a member of
JOE_FAMILY_IDS=$($PSQL -c "
    SELECT DISTINCT family_id FROM family_memberships
    WHERE user_id IN ($(echo "$JOE_USER_IDS" | sed "s/[^,]*/'&'/g"));
" | tr -d ' ' | grep -v '^$' | paste -sd, -)
echo "  family_ids: $JOE_FAMILY_IDS"

# Child IDs in those families
JOE_CHILD_IDS=$($PSQL -c "
    SELECT id FROM children
    WHERE family_id IN ($(echo "$JOE_FAMILY_IDS" | sed "s/[^,]*/'&'/g"));
" | tr -d ' ' | grep -v '^$' | paste -sd, -)
echo "  child_ids: $JOE_CHILD_IDS"

# Sanity stop — if scope is empty, abort BEFORE writing anything
if [ -z "$JOE_USER_IDS" ] || [ -z "$JOE_FAMILY_IDS" ] || [ -z "$JOE_CHILD_IDS" ]; then
    echo "FATAL: Joe scope is empty on $HOST/$DB — refusing to write empty backup"
    exit 1
fi

# Write the manifest first
cat > "$OUTDIR/joe-manifest.json" <<EOF
{
    "host": "$HOST",
    "db": "$DB",
    "extracted_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "user_ids": "$JOE_USER_IDS",
    "family_ids": "$JOE_FAMILY_IDS",
    "child_ids": "$JOE_CHILD_IDS"
}
EOF

# Build comma-separated quoted ID lists for IN clauses
USER_IN=$(echo "$JOE_USER_IDS" | sed "s/[^,]*/'&'/g")
FAMILY_IN=$(echo "$JOE_FAMILY_IDS" | sed "s/[^,]*/'&'/g")
CHILD_IN=$(echo "$JOE_CHILD_IDS" | sed "s/[^,]*/'&'/g")

extract() {
    local name="$1"
    local sql="$2"
    echo "  $name"
    $PSQL --csv -c "$sql" > "$OUTDIR/$name.csv"
}

echo "== extracting Joe's rows from each table =="

# Identity tables
extract users "SELECT * FROM users WHERE id IN ($USER_IN) ORDER BY id;"
extract families "SELECT * FROM families WHERE id IN ($FAMILY_IN) ORDER BY id;"
extract family_memberships "SELECT * FROM family_memberships WHERE user_id IN ($USER_IN) OR family_id IN ($FAMILY_IN) ORDER BY family_id, user_id;"
extract children "SELECT * FROM children WHERE id IN ($CHILD_IN) ORDER BY id;"

# Per-child health/behavior data
for tbl in seizure_logs behavior_logs sleep_logs medication_logs bowel_logs diet_logs sensory_logs social_logs speech_logs therapy_logs weight_logs health_event_logs alerts ai_analysis_log; do
    extract "$tbl" "SELECT * FROM $tbl WHERE child_id IN ($CHILD_IN) ORDER BY id;"
done

# Per-user data (each table's specific user/family/child column)
extract alert_feedback        "SELECT * FROM alert_feedback WHERE user_id IN ($USER_IN) ORDER BY id;"
extract alert_exports         "SELECT * FROM alert_exports WHERE exported_by_user_id IN ($USER_IN) OR shared_with_user_id IN ($USER_IN) ORDER BY id;"
extract correlation_requests  "SELECT * FROM correlation_requests WHERE child_id IN ($CHILD_IN) OR requested_by IN ($USER_IN) ORDER BY id;"
extract notification_preferences  "SELECT * FROM notification_preferences WHERE user_id IN ($USER_IN) OR family_id IN ($FAMILY_IN) ORDER BY user_id, family_id;"
extract user_interaction_preferences "SELECT * FROM user_interaction_preferences WHERE user_id IN ($USER_IN) ORDER BY user_id;"
extract user_subscriptions    "SELECT * FROM user_subscriptions WHERE user_id IN ($USER_IN) ORDER BY id;"
extract device_tokens         "SELECT * FROM device_tokens WHERE user_id IN ($USER_IN) ORDER BY id;"
extract reports               "SELECT * FROM reports WHERE child_id IN ($CHILD_IN) OR family_id IN ($FAMILY_IN) OR created_by IN ($USER_IN) ORDER BY id;"
extract scheduled_reports     "SELECT * FROM scheduled_reports WHERE child_id IN ($CHILD_IN) OR family_id IN ($FAMILY_IN) OR created_by IN ($USER_IN) ORDER BY id;"
extract treatment_changes     "SELECT * FROM treatment_changes WHERE child_id IN ($CHILD_IN) OR changed_by_user_id IN ($USER_IN) ORDER BY id;"
extract treatment_change_responses "SELECT * FROM treatment_change_responses WHERE responded_by_user_id IN ($USER_IN) OR provider_user_id IN ($USER_IN) ORDER BY id;"
extract promo_code_usages     "SELECT * FROM promo_code_usages WHERE user_id IN ($USER_IN) ORDER BY id;"
extract chat_threads          "SELECT * FROM chat_threads WHERE created_by IN ($USER_IN) OR family_id IN ($FAMILY_IN) OR child_id IN ($CHILD_IN) ORDER BY id;"
extract chat_participants     "SELECT * FROM chat_participants WHERE user_id IN ($USER_IN) ORDER BY thread_id, user_id;"
# chat_messages keyed via threads → fetch all messages whose sender is Joe OR whose thread is one of his
extract chat_messages         "SELECT cm.* FROM chat_messages cm WHERE cm.sender_id IN ($USER_IN) OR cm.thread_id IN (SELECT id FROM chat_threads WHERE created_by IN ($USER_IN) OR family_id IN ($FAMILY_IN) OR child_id IN ($CHILD_IN)) ORDER BY cm.id;"
extract family_subscriptions  "SELECT * FROM family_subscriptions WHERE family_id IN ($FAMILY_IN) OR comped_by IN ($USER_IN) ORDER BY id;"
extract password_reset_tokens "SELECT * FROM password_reset_tokens WHERE user_id IN ($USER_IN) ORDER BY id;"
# audit_log + sessions: post-00032 these tables use admin_id + app_user_id
# (split from the old user_id). We adapt at runtime — try the new shape first,
# fall back to the legacy single-column shape so this script works against
# both pre-migration and post-migration databases.
if $PSQL -c "SELECT 1 FROM information_schema.columns WHERE table_name='audit_log' AND column_name='admin_id';" | grep -q 1; then
    extract audit_log "SELECT * FROM audit_log WHERE admin_id IN ($USER_IN) OR app_user_id IN ($USER_IN) OR family_id IN ($FAMILY_IN) ORDER BY id;"
else
    extract audit_log "SELECT * FROM audit_log WHERE user_id IN ($USER_IN) OR family_id IN ($FAMILY_IN) ORDER BY id;"
fi
if $PSQL -c "SELECT 1 FROM information_schema.columns WHERE table_name='sessions' AND column_name='admin_id';" | grep -q 1; then
    extract sessions "SELECT * FROM sessions WHERE admin_id IN ($USER_IN) OR app_user_id IN ($USER_IN) OR family_id IN ($FAMILY_IN) ORDER BY id;"
else
    extract sessions "SELECT * FROM sessions WHERE user_id IN ($USER_IN) OR family_id IN ($FAMILY_IN) ORDER BY id;"
fi
extract payments              "SELECT * FROM payments WHERE user_id IN ($USER_IN) ORDER BY id;"

echo
echo "== row counts =="
for f in "$OUTDIR"/*.csv; do
    echo "  $(basename "$f" .csv): $(wc -l < "$f") rows"
done

echo
echo "Joe extract written to: $OUTDIR"
