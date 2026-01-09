# CareCompanion Scaling Playbook

**Purpose**: Step-by-step procedures for scaling each component without downtime.

---

## Scaling Phases Overview

| Phase | Users | Architecture | Monthly Cost |
|-------|-------|--------------|--------------|
| 1 - Alpha | 1-10 | Single EC2 + RDS + Redis | ~$71 |
| 2 - Beta | 10-100 | Auto Scaling (1-2) + upgraded DB | ~$100 |
| 3 - Production | 100-1000 | ECS Fargate + Multi-AZ + CDN | ~$200-300 |
| 4 - Enterprise | 1000+ | Full HA + Read Replicas | ~$500+ |

---

## Monitoring Thresholds

### When to Scale UP

| Metric | Threshold | Action |
|--------|-----------|--------|
| CPU Utilization | > 70% for 5 min | Add instance |
| Memory Usage | > 80% for 5 min | Add instance |
| Response Time (p95) | > 500ms | Add instance |
| Database Connections | > 80% of max | Upgrade DB |
| Redis Memory | > 80% | Upgrade Redis |
| Disk Usage | > 80% | Increase storage |

### When to Scale DOWN

| Metric | Threshold | Action |
|--------|-----------|--------|
| CPU Utilization | < 30% for 30 min | Remove instance |
| Memory Usage | < 40% for 30 min | Remove instance |

---

## Phase 1 → Phase 2: Adding Auto Scaling

**Trigger**: Sustained CPU > 70% or user count approaching 10

### Step 1: Create Launch Template

```bash
aws ec2 create-launch-template \
  --launch-template-name carecompanion-template \
  --version-description "Initial version" \
  --launch-template-data '{
    "ImageId": "ami-0c7217cdde317cfec",
    "InstanceType": "t3.small",
    "KeyName": "carecompanion-key",
    "SecurityGroupIds": ["<ec2-sg-id>"],
    "IamInstanceProfile": {"Name": "carecompanion-ec2-role"},
    "UserData": "<base64-encoded-user-data>"
  }'
```

### Step 2: Create Auto Scaling Group

```bash
aws autoscaling create-auto-scaling-group \
  --auto-scaling-group-name carecompanion-asg \
  --launch-template LaunchTemplateName=carecompanion-template,Version='$Latest' \
  --min-size 1 \
  --max-size 2 \
  --desired-capacity 1 \
  --vpc-zone-identifier "<public-subnet-a>,<public-subnet-b>" \
  --target-group-arns <target-group-arn> \
  --health-check-type ELB \
  --health-check-grace-period 300
```

### Step 3: Create Scaling Policies

**Scale Out Policy:**
```bash
aws autoscaling put-scaling-policy \
  --auto-scaling-group-name carecompanion-asg \
  --policy-name scale-out \
  --policy-type TargetTrackingScaling \
  --target-tracking-configuration '{
    "PredefinedMetricSpecification": {
      "PredefinedMetricType": "ASGAverageCPUUtilization"
    },
    "TargetValue": 70.0,
    "ScaleOutCooldown": 300,
    "ScaleInCooldown": 300
  }'
```

### Step 4: Upgrade Database

```bash
# Modify instance class (causes brief downtime during maintenance window)
aws rds modify-db-instance \
  --db-instance-identifier carecompanion-db \
  --db-instance-class db.t3.small \
  --apply-immediately
```

### Step 5: Terminate Original EC2

Once ASG is healthy, terminate the original manually-created EC2:
```bash
aws ec2 terminate-instances --instance-ids <original-instance-id>
```

**Estimated Downtime**: 0 (rolling update via ALB)

---

## Phase 2 → Phase 3: Production Ready

**Trigger**: User count > 100 or requiring high availability

### Step 1: Enable Multi-AZ for RDS

```bash
aws rds modify-db-instance \
  --db-instance-identifier carecompanion-db \
  --multi-az \
  --apply-immediately
```

**Note**: This causes a brief failover (~60 seconds).

### Step 2: Upgrade Redis

```bash
# Create new cluster with more capacity
aws elasticache create-cache-cluster \
  --cache-cluster-id carecompanion-redis-v2 \
  --cache-node-type cache.t3.small \
  --engine redis \
  --engine-version 7.0 \
  --num-cache-nodes 1 \
  --cache-subnet-group-name carecompanion-redis-subnet \
  --security-group-ids <redis-sg-id>

# Update application configuration to use new endpoint
# Then delete old cluster after verification
```

### Step 3: Migrate to ECS Fargate

#### 3.1 Create ECS Cluster
```bash
aws ecs create-cluster --cluster-name carecompanion-cluster
```

#### 3.2 Create Task Definition
```json
{
  "family": "carecompanion",
  "networkMode": "awsvpc",
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "512",
  "memory": "1024",
  "containerDefinitions": [{
    "name": "carecompanion",
    "image": "<account-id>.dkr.ecr.us-east-1.amazonaws.com/carecompanion:latest",
    "portMappings": [{
      "containerPort": 8090,
      "protocol": "tcp"
    }],
    "environment": [
      {"name": "DB_HOST", "value": "<rds-endpoint>"},
      {"name": "DB_PORT", "value": "5432"},
      {"name": "REDIS_HOST", "value": "<redis-endpoint>"},
      {"name": "REDIS_PORT", "value": "6379"},
      {"name": "ENVIRONMENT", "value": "production"}
    ],
    "secrets": [
      {"name": "DB_PASSWORD", "valueFrom": "arn:aws:secretsmanager:us-east-1:<account>:secret:carecompanion/db-password"},
      {"name": "JWT_SECRET", "valueFrom": "arn:aws:secretsmanager:us-east-1:<account>:secret:carecompanion/jwt-secret"}
    ],
    "logConfiguration": {
      "logDriver": "awslogs",
      "options": {
        "awslogs-group": "/ecs/carecompanion",
        "awslogs-region": "us-east-1",
        "awslogs-stream-prefix": "ecs"
      }
    }
  }]
}
```

#### 3.3 Create ECS Service
```bash
aws ecs create-service \
  --cluster carecompanion-cluster \
  --service-name carecompanion-service \
  --task-definition carecompanion:1 \
  --desired-count 2 \
  --launch-type FARGATE \
  --network-configuration '{
    "awsvpcConfiguration": {
      "subnets": ["<private-subnet-a>", "<private-subnet-b>"],
      "securityGroups": ["<ec2-sg-id>"],
      "assignPublicIp": "DISABLED"
    }
  }' \
  --load-balancers '[{
    "targetGroupArn": "<target-group-arn>",
    "containerName": "carecompanion",
    "containerPort": 8090
  }]'
```

#### 3.4 Configure Auto Scaling for ECS
```bash
# Register scalable target
aws application-autoscaling register-scalable-target \
  --service-namespace ecs \
  --scalable-dimension ecs:service:DesiredCount \
  --resource-id service/carecompanion-cluster/carecompanion-service \
  --min-capacity 2 \
  --max-capacity 10

# Create scaling policy
aws application-autoscaling put-scaling-policy \
  --service-namespace ecs \
  --scalable-dimension ecs:service:DesiredCount \
  --resource-id service/carecompanion-cluster/carecompanion-service \
  --policy-name cpu-scaling \
  --policy-type TargetTrackingScaling \
  --target-tracking-scaling-policy-configuration '{
    "PredefinedMetricSpecification": {
      "PredefinedMetricType": "ECSServiceAverageCPUUtilization"
    },
    "TargetValue": 70.0,
    "ScaleOutCooldown": 60,
    "ScaleInCooldown": 300
  }'
```

### Step 4: Add CloudFront CDN

```bash
aws cloudfront create-distribution \
  --origin-domain-name <alb-dns-name> \
  --default-root-object "" \
  --comment "CareCompanion CDN"
```

Configure cache behaviors:
- Static assets (*.css, *.js, images): Cache for 1 week
- API endpoints: No caching
- HTML pages: Short TTL or no cache

### Step 5: Clean Up EC2 Auto Scaling Group

Once ECS is stable, delete the ASG:
```bash
aws autoscaling delete-auto-scaling-group \
  --auto-scaling-group-name carecompanion-asg \
  --force-delete
```

**Estimated Downtime**: 0 (blue-green via ALB)

---

## Phase 3 → Phase 4: Enterprise Scale

**Trigger**: User count > 1000 or strict SLA requirements

### Step 1: Upgrade Database to Larger Instance

```bash
aws rds modify-db-instance \
  --db-instance-identifier carecompanion-db \
  --db-instance-class db.r6g.large \
  --apply-immediately
```

### Step 2: Add Read Replicas

```bash
aws rds create-db-instance-read-replica \
  --db-instance-identifier carecompanion-db-replica-1 \
  --source-db-instance-identifier carecompanion-db \
  --db-instance-class db.r6g.large

# Update application to use read replica for read queries
```

### Step 3: Enable Redis Cluster Mode

```bash
# Create cluster with multiple shards
aws elasticache create-replication-group \
  --replication-group-id carecompanion-redis-cluster \
  --replication-group-description "CareCompanion Redis Cluster" \
  --cache-node-type cache.r6g.large \
  --num-node-groups 2 \
  --replicas-per-node-group 1 \
  --automatic-failover-enabled \
  --cache-subnet-group-name carecompanion-redis-subnet \
  --security-group-ids <redis-sg-id>
```

### Step 4: Consider Aurora PostgreSQL Migration

For very high scale, migrate from RDS PostgreSQL to Aurora:
1. Create Aurora cluster from RDS snapshot
2. Test application with Aurora endpoint
3. Switch DNS/configuration with minimal downtime

---

## Zero-Downtime Deployment Procedures

### Blue-Green Deployment (Recommended)

1. **Deploy "Green" Environment**
   ```bash
   # Update task definition with new image
   aws ecs register-task-definition --cli-input-json file://task-definition.json

   # Create new service with updated task
   aws ecs create-service \
     --cluster carecompanion-cluster \
     --service-name carecompanion-green \
     --task-definition carecompanion:2 \
     --desired-count 2 \
     ...
   ```

2. **Run Health Checks**
   - Wait for green service to be healthy
   - Run smoke tests against green service directly

3. **Switch Traffic**
   ```bash
   # Update ALB to point to green target group
   aws elbv2 modify-listener \
     --listener-arn <listener-arn> \
     --default-actions Type=forward,TargetGroupArn=<green-target-group>
   ```

4. **Monitor & Rollback Window**
   - Watch metrics for 10 minutes
   - If issues: switch back to blue target group

5. **Cleanup**
   ```bash
   # Delete old (blue) service
   aws ecs delete-service \
     --cluster carecompanion-cluster \
     --service carecompanion-blue \
     --force
   ```

### Rolling Update (ECS Default)

Configure service to maintain availability during updates:
```bash
aws ecs update-service \
  --cluster carecompanion-cluster \
  --service carecompanion-service \
  --deployment-configuration '{
    "minimumHealthyPercent": 100,
    "maximumPercent": 200
  }'
```

This ensures:
- New tasks start before old tasks stop
- Always at least 100% capacity available
- Brief overlap with 200% capacity during deploy

---

## Rollback Procedures

### Application Rollback

```bash
# Get previous task definition
aws ecs describe-task-definition --task-definition carecompanion:<previous-version>

# Update service to use previous version
aws ecs update-service \
  --cluster carecompanion-cluster \
  --service carecompanion-service \
  --task-definition carecompanion:<previous-version>
```

### Database Rollback

```bash
# Restore from snapshot
aws rds restore-db-instance-from-db-snapshot \
  --db-instance-identifier carecompanion-db-restored \
  --db-snapshot-identifier <snapshot-id>

# Update application to use restored instance
# (or rename instances)
```

### Redis Rollback

Redis data is ephemeral (sessions). If needed:
1. Create new cluster
2. Update application endpoint
3. Users will need to re-login

---

## Cost Optimization Tips

1. **Use Reserved Instances**: For predictable workloads, reserve EC2/RDS for 1-3 years (up to 72% savings)

2. **Spot Instances**: For non-critical workloads, use Spot for ~90% savings

3. **Right-size**: Regularly review CloudWatch metrics and downsize over-provisioned resources

4. **S3 Lifecycle Policies**: Move old uploads to S3 Glacier after 90 days

5. **RDS Storage Autoscaling**: Enable to avoid over-provisioning storage

---

## Monitoring Setup

### CloudWatch Alarms

```bash
# High CPU alarm
aws cloudwatch put-metric-alarm \
  --alarm-name carecompanion-high-cpu \
  --metric-name CPUUtilization \
  --namespace AWS/ECS \
  --statistic Average \
  --period 300 \
  --threshold 80 \
  --comparison-operator GreaterThanThreshold \
  --evaluation-periods 2 \
  --alarm-actions <sns-topic-arn>

# High memory alarm
aws cloudwatch put-metric-alarm \
  --alarm-name carecompanion-high-memory \
  --metric-name MemoryUtilization \
  --namespace AWS/ECS \
  --statistic Average \
  --period 300 \
  --threshold 85 \
  --comparison-operator GreaterThanThreshold \
  --evaluation-periods 2 \
  --alarm-actions <sns-topic-arn>

# Database connections alarm
aws cloudwatch put-metric-alarm \
  --alarm-name carecompanion-db-connections \
  --metric-name DatabaseConnections \
  --namespace AWS/RDS \
  --statistic Average \
  --period 300 \
  --threshold 80 \
  --comparison-operator GreaterThanThreshold \
  --evaluation-periods 2 \
  --alarm-actions <sns-topic-arn>
```

### Dashboard

Create a CloudWatch dashboard with:
- ECS CPU/Memory
- RDS Connections/CPU
- Redis Memory/Connections
- ALB Request Count/Latency
- 4xx/5xx Error Rates
