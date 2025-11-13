#!/bin/bash

#############################################################################
# CPLS Backend - Simple 2-Step Deployment
#############################################################################
# Step 1: Build & Push image
# Step 2: Deploy to Cloud Run with environment variables
#############################################################################

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}================================================${NC}"
echo -e "${BLUE}CPLS Backend - Cloud Run Deployment${NC}"
echo -e "${BLUE}================================================${NC}\n"

# Get project ID
PROJECT_ID=$(gcloud config get-value project 2>/dev/null)
if [ -z "$PROJECT_ID" ] || [ "$PROJECT_ID" == "(unset)" ]; then
    echo -e "${RED}❌ No project configured${NC}"
    echo "Run: gcloud config set project YOUR_PROJECT_ID"
    exit 1
fi

echo -e "${GREEN}✅ Project: $PROJECT_ID${NC}"
echo -e "${BLUE}Region: asia-southeast1${NC}\n"

#############################################################################
# Get Supabase Password
#############################################################################

echo -e "${YELLOW}Enter your Supabase database password:${NC}"
read -s DB_PASSWORD
echo ""

if [ -z "$DB_PASSWORD" ]; then
    echo -e "${RED}❌ Password cannot be empty${NC}"
    exit 1
fi

# Generate JWT secret
JWT_SECRET=$(openssl rand -base64 32)

echo -e "${GREEN}✅ Credentials collected${NC}\n"

#############################################################################
# Step 1: Build and Push Image
#############################################################################

echo -e "${BLUE}📦 Step 1: Building Docker image...${NC}\n"

gcloud builds submit --config cloudbuild.yaml

echo -e "\n${GREEN}✅ Image built and pushed${NC}\n"

#############################################################################
# Step 2: Deploy to Cloud Run
#############################################################################

echo -e "${BLUE}🚀 Step 2: Deploying to Cloud Run...${NC}\n"

gcloud run deploy cpls-be \
  --image=gcr.io/$PROJECT_ID/cpls-be \
  --region=asia-southeast1 \
  --platform=managed \
  --allow-unauthenticated \
  --memory=512Mi \
  --cpu=1 \
  --max-instances=10 \
  --min-instances=0 \
  --timeout=600 \
  --port=8080 \
  --set-env-vars="DB_HOST=db.lqmocewqozpyzsknocrc.supabase.co,DB_PORT=5432,DB_USER=postgres,DB_PASSWORD=${DB_PASSWORD},DB_NAME=postgres,JWT_SECRET=${JWT_SECRET},ENVIRONMENT=production"

#############################################################################
# Get Service URL
#############################################################################

echo -e "\n${BLUE}================================================${NC}"
echo -e "${BLUE}✅ Deployment Complete!${NC}"
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
    echo -e "${BLUE}Testing health endpoint...${NC}"
    sleep 5

    HEALTH_RESPONSE=$(curl -s "${SERVICE_URL}/health" 2>/dev/null || echo "failed")
    if echo "$HEALTH_RESPONSE" | grep -q "ok"; then
        echo -e "${GREEN}✅ Service is running!${NC}\n"
    else
        echo -e "${YELLOW}⚠️  Health check inconclusive${NC}"
        echo -e "Response: $HEALTH_RESPONSE"
        echo -e "\nCheck logs:"
        echo -e "  gcloud run services logs tail cpls-be --region=asia-southeast1\n"
    fi
fi

echo -e "${GREEN}🎉 Done!${NC}\n"
