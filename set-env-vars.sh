#!/bin/bash

#############################################################################
# Set Environment Variables for Existing Cloud Run Service
#############################################################################
# This script updates the cpls-be service with required env vars
# Use this if you already deployed but forgot to set env vars
#############################################################################

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}================================================${NC}"
echo -e "${BLUE}Set Environment Variables for cpls-be${NC}"
echo -e "${BLUE}================================================${NC}\n"

# Check service exists
if ! gcloud run services describe cpls-be --region=asia-southeast1 &>/dev/null; then
    echo -e "${RED}❌ Service 'cpls-be' not found in asia-southeast1${NC}"
    echo "Please deploy the service first"
    exit 1
fi

echo -e "${GREEN}✅ Found service: cpls-be${NC}\n"

#############################################################################
# Get Database Password
#############################################################################

echo -e "${YELLOW}📝 Database Configuration${NC}\n"
echo -e "${BLUE}Enter your Supabase database password:${NC}"
echo -e "   (Supabase Dashboard → Settings → Database → Database Password)"
read -s DB_PASSWORD
echo ""

if [ -z "$DB_PASSWORD" ]; then
    echo -e "${RED}❌ Password cannot be empty${NC}"
    exit 1
fi

# Generate JWT Secret
echo -e "${BLUE}Generating JWT secret...${NC}"
JWT_SECRET=$(openssl rand -base64 32)
echo -e "${GREEN}✅ JWT secret generated${NC}\n"

#############################################################################
# Confirm
#############################################################################

echo -e "${YELLOW}📋 Configuration to apply:${NC}"
echo "  DB_HOST: db.lqmocewqozpyzsknocrc.supabase.co"
echo "  DB_PORT: 5432"
echo "  DB_USER: postgres"
echo "  DB_PASSWORD: ********"
echo "  DB_NAME: postgres"
echo "  JWT_SECRET: ********"
echo "  ENVIRONMENT: production"
echo ""

read -p "Apply these settings? (y/N): " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${YELLOW}Cancelled${NC}"
    exit 0
fi

#############################################################################
# Update Service
#############################################################################

echo -e "\n${BLUE}🔧 Updating service with environment variables...${NC}\n"

gcloud run services update cpls-be \
  --region=asia-southeast1 \
  --update-env-vars="DB_HOST=db.lqmocewqozpyzsknocrc.supabase.co,DB_PORT=5432,DB_USER=postgres,DB_PASSWORD=${DB_PASSWORD},DB_NAME=postgres,JWT_SECRET=${JWT_SECRET},ENVIRONMENT=production"

#############################################################################
# Get Service URL
#############################################################################

echo -e "\n${BLUE}================================================${NC}"
echo -e "${BLUE}✅ Environment Variables Set!${NC}"
echo -e "${BLUE}================================================${NC}\n"

SERVICE_URL=$(gcloud run services describe cpls-be \
    --region=asia-southeast1 \
    --format="value(status.url)" 2>/dev/null)

if [ -n "$SERVICE_URL" ]; then
    echo -e "${GREEN}📍 Service URL:${NC}  $SERVICE_URL"
    echo -e "${GREEN}🔧 Admin UI:${NC}    $SERVICE_URL/admin"
    echo -e "${GREEN}🏥 Health:${NC}      $SERVICE_URL/health"
    echo ""

    # Test health
    echo -e "${BLUE}Waiting for new revision to become ready...${NC}"
    sleep 10

    echo -e "${BLUE}Testing health endpoint...${NC}"
    HEALTH_RESPONSE=$(curl -s "${SERVICE_URL}/health" 2>/dev/null || echo "failed")

    if echo "$HEALTH_RESPONSE" | grep -q "ok"; then
        echo -e "${GREEN}✅ Service is healthy!${NC}"
        echo -e "${GREEN}Response:${NC} $HEALTH_RESPONSE\n"
    else
        echo -e "${YELLOW}⚠️  Health check response:${NC} $HEALTH_RESPONSE"
        echo -e "\nIf still failing, check logs:"
        echo -e "  gcloud run services logs tail cpls-be --region=asia-southeast1\n"
    fi
fi

echo -e "${GREEN}🎉 Done!${NC}\n"
