#!/bin/bash
set -e

cd "$(dirname "$0")/.."

# =============================================================================
# CareCompanion Production Deploy Script
#
# SAFETY: This script requires explicit confirmation before every
# production-impacting action. This safeguard is designed for the beta period.
#
# To remove this safeguard after beta:
#   Set CARECOMPANION_SKIP_DEPLOY_CONFIRM=1 in your environment
#   or delete the confirm_production() function and its calls.
# =============================================================================

REGION="us-east-1"
ECR_URI="943431294725.dkr.ecr.us-east-1.amazonaws.com/carecompanion"
ASG_NAME="carecompanion-asg"
DEPLOY_LOG="deployments.log"

# Colors
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

log_deployment() {
    local action="$1"
    local status="$2"
    echo "$(date -u '+%Y-%m-%d %H:%M:%S UTC') | $action | $status | user=$(whoami)" >> "$DEPLOY_LOG"
}

confirm_production() {
    local action="$1"

    # Skip confirmation if explicitly opted out (post-beta)
    if [ "${CARECOMPANION_SKIP_DEPLOY_CONFIRM}" = "1" ]; then
        return 0
    fi

    echo ""
    echo -e "${RED}╔══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║              ⚠  PRODUCTION ACTION  ⚠                   ║${NC}"
    echo -e "${RED}╠══════════════════════════════════════════════════════════╣${NC}"
    echo -e "${RED}║${NC}  Action: ${YELLOW}${action}${NC}"
    echo -e "${RED}║${NC}"
    echo -e "${RED}║${NC}  This will affect the LIVE production environment at"
    echo -e "${RED}║${NC}  ${YELLOW}https://www.mycarecompanion.net${NC}"
    echo -e "${RED}║${NC}"
    echo -e "${RED}║${NC}  Type ${GREEN}DEPLOY${NC} to confirm, or anything else to abort."
    echo -e "${RED}╚══════════════════════════════════════════════════════════╝${NC}"
    echo ""
    read -p "Confirm: " response

    if [ "$response" != "DEPLOY" ]; then
        echo ""
        echo -e "${YELLOW}Aborted.${NC} No changes made to production."
        log_deployment "$action" "ABORTED by user"
        exit 1
    fi

    log_deployment "$action" "CONFIRMED by user"
}

echo ""
echo -e "${GREEN}=== CareCompanion Production Deploy ===${NC}"
echo ""
echo "This will:"
echo "  1. Build a Docker image from current code"
echo "  2. Push it to ECR"
echo "  3. Trigger an ASG instance refresh (zero-downtime)"
echo ""

# Step 1: Build
confirm_production "Docker Build (create production image)"

echo ""
echo "1/4 Building Docker image..."
sudo docker build -t carecompanion:latest .
log_deployment "Docker Build" "SUCCESS"
echo -e "${GREEN}Build complete.${NC}"

# Step 2: Push to ECR
confirm_production "ECR Push (upload image to production registry)"

echo ""
echo "2/4 Logging in to ECR..."
aws ecr get-login-password --region $REGION | sudo docker login --username AWS --password-stdin ${ECR_URI%/*}

echo "    Pushing to ECR..."
sudo docker tag carecompanion:latest ${ECR_URI}:latest
sudo docker push ${ECR_URI}:latest
log_deployment "ECR Push" "SUCCESS"
echo -e "${GREEN}Push complete.${NC}"

# Step 3: Deploy
confirm_production "ASG Instance Refresh (replace running production servers)"

echo ""
echo "3/4 Starting instance refresh..."
REFRESH_ID=$(aws autoscaling start-instance-refresh \
    --auto-scaling-group-name $ASG_NAME \
    --preferences '{"MinHealthyPercentage":0,"InstanceWarmup":120}' \
    --region $REGION \
    --query 'InstanceRefreshId' --output text)
log_deployment "ASG Instance Refresh" "STARTED refresh_id=$REFRESH_ID"

echo -e "${GREEN}4/4 Instance refresh started: ${REFRESH_ID}${NC}"
echo ""
echo "Monitor with:"
echo "  aws autoscaling describe-instance-refreshes --auto-scaling-group-name $ASG_NAME --region $REGION --query 'InstanceRefreshes[0].[Status,PercentageComplete]' --output table"
echo ""
echo "Check target health:"
echo "  aws elbv2 describe-target-health --region $REGION --target-group-arn arn:aws:elasticloadbalancing:us-east-1:943431294725:targetgroup/carecompanion-tg/bade3e56ae036ce7 --query 'TargetHealthDescriptions[*].[Target.Id,TargetHealth.State]' --output table"
echo ""
echo -e "${GREEN}Deploy triggered. New instance will be live in ~2-3 minutes.${NC}"
log_deployment "Full Deploy" "COMPLETE"
