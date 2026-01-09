# AWS Deployment Guide for CareCompanion

**Target Architecture**: Small Tier (~$71/month)
**Target Environment**: Production Alpha

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                         AWS Cloud                            │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │                     Public Subnets                       │ │
│  │  ┌──────────────┐    ┌──────────────────────────────┐   │ │
│  │  │   Route53    │───▶│  Application Load Balancer   │   │ │
│  │  │   (Domain)   │    │  (HTTPS via ACM Certificate) │   │ │
│  │  └──────────────┘    └──────────────┬───────────────┘   │ │
│  └──────────────────────────────────────┼───────────────────┘ │
│                                         │                     │
│  ┌──────────────────────────────────────┼───────────────────┐ │
│  │                   Private Subnets    ▼                   │ │
│  │  ┌────────────────────────────────────────────────────┐  │ │
│  │  │           Auto Scaling Group (min:1, max:2)        │  │ │
│  │  │  ┌─────────────────┐    ┌─────────────────┐        │  │ │
│  │  │  │   EC2 t3.small  │    │  EC2 t3.small   │        │  │ │
│  │  │  │   (Go Server)   │    │   (standby)     │        │  │ │
│  │  │  └────────┬────────┘    └────────┬────────┘        │  │ │
│  │  └───────────┼──────────────────────┼─────────────────┘  │ │
│  │              │                      │                    │ │
│  │  ┌───────────▼──────────────────────▼─────────────────┐  │ │
│  │  │                 Data Layer                          │  │ │
│  │  │  ┌─────────────────┐    ┌─────────────────┐        │  │ │
│  │  │  │ RDS PostgreSQL  │    │ ElastiCache     │        │  │ │
│  │  │  │ db.t3.small     │    │ cache.t3.micro  │        │  │ │
│  │  │  │ (20GB gp3)      │    │ (Redis 7.x)     │        │  │ │
│  │  │  └─────────────────┘    └─────────────────┘        │  │ │
│  │  └────────────────────────────────────────────────────┘  │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                               │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │  S3 Bucket (uploads) │ Secrets Manager │ CloudWatch Logs │ │
│  └──────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

---

## Cost Breakdown (Estimated Monthly)

| Service | Specification | Est. Cost |
|---------|--------------|-----------|
| EC2 t3.small | 2 vCPU, 2GB RAM, on-demand | ~$15/month |
| RDS db.t3.small | PostgreSQL 16, 20GB gp3 | ~$25/month |
| ElastiCache | cache.t3.micro, Redis 7.x | ~$12/month |
| ALB | Application Load Balancer | ~$16/month |
| S3 | 10GB storage + requests | ~$1/month |
| Route53 | Hosted zone + queries | ~$1/month |
| Secrets Manager | 3 secrets | ~$1.20/month |
| **Total** | | **~$71/month** |

---

## Prerequisites

1. AWS Account with admin access
2. AWS CLI installed and configured
3. Domain name (for SSL certificate)
4. Docker installed locally (for building images)

---

## Step 1: Initial AWS Setup

### 1.1 Configure AWS CLI
```bash
aws configure
# Enter your Access Key ID
# Enter your Secret Access Key
# Default region: us-east-1
# Default output format: json
```

### 1.2 Create VPC (if not using default)
```bash
aws ec2 create-vpc --cidr-block 10.0.0.0/16 --tag-specifications 'ResourceType=vpc,Tags=[{Key=Name,Value=carecompanion-vpc}]'
```

Save the VPC ID returned.

### 1.3 Create Subnets
```bash
# Public Subnet A (us-east-1a)
aws ec2 create-subnet --vpc-id <vpc-id> --cidr-block 10.0.1.0/24 --availability-zone us-east-1a

# Public Subnet B (us-east-1b)
aws ec2 create-subnet --vpc-id <vpc-id> --cidr-block 10.0.2.0/24 --availability-zone us-east-1b

# Private Subnet A (us-east-1a)
aws ec2 create-subnet --vpc-id <vpc-id> --cidr-block 10.0.10.0/24 --availability-zone us-east-1a

# Private Subnet B (us-east-1b)
aws ec2 create-subnet --vpc-id <vpc-id> --cidr-block 10.0.11.0/24 --availability-zone us-east-1b
```

---

## Step 2: Create Security Groups

### 2.1 ALB Security Group
```bash
aws ec2 create-security-group \
  --group-name carecompanion-alb-sg \
  --description "ALB Security Group" \
  --vpc-id <vpc-id>

# Allow HTTP
aws ec2 authorize-security-group-ingress \
  --group-id <alb-sg-id> \
  --protocol tcp --port 80 --cidr 0.0.0.0/0

# Allow HTTPS
aws ec2 authorize-security-group-ingress \
  --group-id <alb-sg-id> \
  --protocol tcp --port 443 --cidr 0.0.0.0/0
```

### 2.2 EC2 Security Group
```bash
aws ec2 create-security-group \
  --group-name carecompanion-ec2-sg \
  --description "EC2 Security Group" \
  --vpc-id <vpc-id>

# Allow from ALB only
aws ec2 authorize-security-group-ingress \
  --group-id <ec2-sg-id> \
  --protocol tcp --port 8090 --source-group <alb-sg-id>

# Allow SSH (restrict to your IP in production)
aws ec2 authorize-security-group-ingress \
  --group-id <ec2-sg-id> \
  --protocol tcp --port 22 --cidr <your-ip>/32
```

### 2.3 RDS Security Group
```bash
aws ec2 create-security-group \
  --group-name carecompanion-rds-sg \
  --description "RDS Security Group" \
  --vpc-id <vpc-id>

# Allow from EC2 only
aws ec2 authorize-security-group-ingress \
  --group-id <rds-sg-id> \
  --protocol tcp --port 5432 --source-group <ec2-sg-id>
```

### 2.4 ElastiCache Security Group
```bash
aws ec2 create-security-group \
  --group-name carecompanion-redis-sg \
  --description "Redis Security Group" \
  --vpc-id <vpc-id>

# Allow from EC2 only
aws ec2 authorize-security-group-ingress \
  --group-id <redis-sg-id> \
  --protocol tcp --port 6379 --source-group <ec2-sg-id>
```

---

## Step 3: Create Database (RDS)

### 3.1 Create DB Subnet Group
```bash
aws rds create-db-subnet-group \
  --db-subnet-group-name carecompanion-db-subnet \
  --db-subnet-group-description "CareCompanion DB Subnets" \
  --subnet-ids <private-subnet-a-id> <private-subnet-b-id>
```

### 3.2 Create RDS Instance
```bash
aws rds create-db-instance \
  --db-instance-identifier carecompanion-db \
  --db-instance-class db.t3.small \
  --engine postgres \
  --engine-version 16 \
  --master-username carecompanion \
  --master-user-password <SECURE_PASSWORD_HERE> \
  --allocated-storage 20 \
  --storage-type gp3 \
  --vpc-security-group-ids <rds-sg-id> \
  --db-subnet-group-name carecompanion-db-subnet \
  --db-name carecompanion \
  --backup-retention-period 7 \
  --no-publicly-accessible \
  --no-multi-az
```

Wait for the instance to be available (~10 minutes):
```bash
aws rds wait db-instance-available --db-instance-identifier carecompanion-db
```

Get the endpoint:
```bash
aws rds describe-db-instances --db-instance-identifier carecompanion-db \
  --query 'DBInstances[0].Endpoint.Address' --output text
```

---

## Step 4: Create Redis (ElastiCache)

### 4.1 Create Subnet Group
```bash
aws elasticache create-cache-subnet-group \
  --cache-subnet-group-name carecompanion-redis-subnet \
  --cache-subnet-group-description "CareCompanion Redis Subnets" \
  --subnet-ids <private-subnet-a-id> <private-subnet-b-id>
```

### 4.2 Create Redis Cluster
```bash
aws elasticache create-cache-cluster \
  --cache-cluster-id carecompanion-redis \
  --cache-node-type cache.t3.micro \
  --engine redis \
  --engine-version 7.0 \
  --num-cache-nodes 1 \
  --cache-subnet-group-name carecompanion-redis-subnet \
  --security-group-ids <redis-sg-id>
```

Get the endpoint:
```bash
aws elasticache describe-cache-clusters --cache-cluster-id carecompanion-redis \
  --show-cache-node-info \
  --query 'CacheClusters[0].CacheNodes[0].Endpoint.Address' --output text
```

---

## Step 5: Create S3 Bucket

```bash
aws s3 mb s3://carecompanion-uploads-<unique-suffix> --region us-east-1

# Enable versioning
aws s3api put-bucket-versioning \
  --bucket carecompanion-uploads-<unique-suffix> \
  --versioning-configuration Status=Enabled

# Block public access
aws s3api put-public-access-block \
  --bucket carecompanion-uploads-<unique-suffix> \
  --public-access-block-configuration \
    BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
```

---

## Step 6: Store Secrets

```bash
# Database password
aws secretsmanager create-secret \
  --name carecompanion/db-password \
  --secret-string "<DB_PASSWORD>"

# JWT secret
aws secretsmanager create-secret \
  --name carecompanion/jwt-secret \
  --secret-string "<JWT_SECRET>"

# Redis password (if using AUTH)
aws secretsmanager create-secret \
  --name carecompanion/redis-password \
  --secret-string "<REDIS_PASSWORD>"
```

---

## Step 7: Request SSL Certificate

### 7.1 Request Certificate
```bash
aws acm request-certificate \
  --domain-name your-domain.com \
  --validation-method DNS \
  --region us-east-1
```

### 7.2 Validate Certificate
Add the CNAME record shown in the ACM console to your DNS provider.

Wait for validation:
```bash
aws acm wait certificate-validated --certificate-arn <cert-arn>
```

---

## Step 8: Build and Push Docker Image

### 8.1 Create ECR Repository
```bash
aws ecr create-repository --repository-name carecompanion
```

### 8.2 Build Image
```bash
cd /home/carecomp/carecompanion
docker build -t carecompanion .
```

### 8.3 Push to ECR
```bash
# Login to ECR
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin <account-id>.dkr.ecr.us-east-1.amazonaws.com

# Tag image
docker tag carecompanion:latest <account-id>.dkr.ecr.us-east-1.amazonaws.com/carecompanion:latest

# Push
docker push <account-id>.dkr.ecr.us-east-1.amazonaws.com/carecompanion:latest
```

---

## Step 9: Create EC2 Instance

### 9.1 Create IAM Role
Create a role with these policies:
- AmazonEC2ContainerRegistryReadOnly
- AmazonS3FullAccess (or scoped to your bucket)
- SecretsManagerReadWrite (or scoped to your secrets)

### 9.2 Create Key Pair
```bash
aws ec2 create-key-pair --key-name carecompanion-key --query 'KeyMaterial' --output text > carecompanion-key.pem
chmod 400 carecompanion-key.pem
```

### 9.3 Launch Instance
```bash
aws ec2 run-instances \
  --image-id ami-0c7217cdde317cfec \
  --instance-type t3.small \
  --key-name carecompanion-key \
  --security-group-ids <ec2-sg-id> \
  --subnet-id <public-subnet-a-id> \
  --iam-instance-profile Name=carecompanion-ec2-role \
  --user-data file://scripts/user-data.sh \
  --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=carecompanion-server}]'
```

### 9.4 User Data Script (`scripts/user-data.sh`)
```bash
#!/bin/bash
yum update -y
yum install -y docker
systemctl start docker
systemctl enable docker

# Login to ECR
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin <account-id>.dkr.ecr.us-east-1.amazonaws.com

# Pull and run container
docker pull <account-id>.dkr.ecr.us-east-1.amazonaws.com/carecompanion:latest

docker run -d \
  --name carecompanion \
  --restart always \
  -p 8090:8090 \
  -e DB_HOST=<rds-endpoint> \
  -e DB_PORT=5432 \
  -e DB_USER=carecompanion \
  -e DB_PASSWORD=<from-secrets-manager> \
  -e DB_NAME=carecompanion \
  -e REDIS_HOST=<redis-endpoint> \
  -e REDIS_PORT=6379 \
  -e JWT_SECRET=<from-secrets-manager> \
  -e ENVIRONMENT=production \
  <account-id>.dkr.ecr.us-east-1.amazonaws.com/carecompanion:latest
```

---

## Step 10: Create Load Balancer

### 10.1 Create Target Group
```bash
aws elbv2 create-target-group \
  --name carecompanion-tg \
  --protocol HTTP \
  --port 8090 \
  --vpc-id <vpc-id> \
  --health-check-path /health \
  --health-check-interval-seconds 30 \
  --healthy-threshold-count 2 \
  --unhealthy-threshold-count 3
```

### 10.2 Register EC2 Instance
```bash
aws elbv2 register-targets \
  --target-group-arn <target-group-arn> \
  --targets Id=<instance-id>
```

### 10.3 Create ALB
```bash
aws elbv2 create-load-balancer \
  --name carecompanion-alb \
  --subnets <public-subnet-a-id> <public-subnet-b-id> \
  --security-groups <alb-sg-id> \
  --scheme internet-facing \
  --type application
```

### 10.4 Create HTTPS Listener
```bash
aws elbv2 create-listener \
  --load-balancer-arn <alb-arn> \
  --protocol HTTPS \
  --port 443 \
  --certificates CertificateArn=<acm-cert-arn> \
  --default-actions Type=forward,TargetGroupArn=<target-group-arn>
```

### 10.5 Create HTTP to HTTPS Redirect
```bash
aws elbv2 create-listener \
  --load-balancer-arn <alb-arn> \
  --protocol HTTP \
  --port 80 \
  --default-actions Type=redirect,RedirectConfig='{Protocol=HTTPS,Port=443,StatusCode=HTTP_301}'
```

---

## Step 11: Configure DNS (Route53)

### 11.1 Create Hosted Zone (if not exists)
```bash
aws route53 create-hosted-zone \
  --name your-domain.com \
  --caller-reference $(date +%s)
```

### 11.2 Create A Record
```bash
aws route53 change-resource-record-sets \
  --hosted-zone-id <zone-id> \
  --change-batch '{
    "Changes": [{
      "Action": "CREATE",
      "ResourceRecordSet": {
        "Name": "your-domain.com",
        "Type": "A",
        "AliasTarget": {
          "HostedZoneId": "<alb-hosted-zone-id>",
          "DNSName": "<alb-dns-name>",
          "EvaluateTargetHealth": true
        }
      }
    }]
  }'
```

---

## Step 12: Run Database Migrations

SSH into EC2 instance and run migrations:
```bash
ssh -i carecompanion-key.pem ec2-user@<ec2-public-ip>

# Connect to container
docker exec -it carecompanion /bin/sh

# Run migrations (adjust for your migration tool)
./carecompanion migrate up
```

Or connect to RDS directly:
```bash
psql -h <rds-endpoint> -U carecompanion -d carecompanion -f migrations/001_initial.sql
```

---

## Verification Checklist

- [ ] ALB health checks passing
- [ ] HTTPS working with valid certificate
- [ ] Can access login page
- [ ] Can login with test credentials
- [ ] Database queries working
- [ ] Redis session management working
- [ ] File uploads working (S3)
- [ ] Logs appearing in CloudWatch

---

## Troubleshooting

### ALB Health Checks Failing
1. Check security group allows ALB to reach EC2 on port 8090
2. Verify `/health` endpoint returns 200
3. Check EC2 instance is running

### Cannot Connect to Database
1. Check RDS security group allows EC2
2. Verify DB credentials in Secrets Manager
3. Check RDS instance is available

### SSL Certificate Issues
1. Ensure certificate is validated
2. Check certificate is in us-east-1 (required for ALB)
3. Verify domain matches certificate

### Application Errors
1. Check CloudWatch logs: `/aws/ec2/carecompanion`
2. SSH to EC2 and check Docker logs: `docker logs carecompanion`
3. Verify environment variables are set correctly

---

## Maintenance Commands

### Update Application
```bash
# Build and push new image
docker build -t carecompanion .
docker tag carecompanion:latest <account-id>.dkr.ecr.us-east-1.amazonaws.com/carecompanion:latest
docker push <account-id>.dkr.ecr.us-east-1.amazonaws.com/carecompanion:latest

# SSH to EC2 and pull new image
ssh -i carecompanion-key.pem ec2-user@<ec2-ip>
docker pull <account-id>.dkr.ecr.us-east-1.amazonaws.com/carecompanion:latest
docker stop carecompanion
docker rm carecompanion
# Run new container (use same run command as user-data.sh)
```

### Database Backup
```bash
# Manual snapshot
aws rds create-db-snapshot \
  --db-instance-identifier carecompanion-db \
  --db-snapshot-identifier carecompanion-manual-$(date +%Y%m%d)
```

### View Logs
```bash
# EC2 application logs
ssh -i carecompanion-key.pem ec2-user@<ec2-ip>
docker logs -f carecompanion

# CloudWatch logs (if configured)
aws logs tail /aws/ec2/carecompanion --follow
```
