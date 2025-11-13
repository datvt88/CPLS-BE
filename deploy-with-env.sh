#!/bin/bash

#############################################################################
# CPLS Backend - Deploy with Environment Variables
#############################################################################
# This script helps you deploy to Cloud Run with proper database credentials
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

# Check if in correct directory
if [ ! -f "cloudbuild.yaml" ]; then
    echo -e "${RED}❌ cloudbuild.yaml not found!${NC}"
    echo "Please run this script from the CPLS-BE directory"
    exit 1
fi

# Get project ID
PROJECT_ID=$(gcloud config get-value project 2>/dev/null)
if [ -z "$PROJECT_ID" ] || [ "$PROJECT_ID" == "(unset)" ]; then
    echo -e "${RED}❌ No Google Cloud project configured${NC}"
    echo "Run: gcloud config set project YOUR_PROJECT_ID"
    exit 1
fi

echo -e "${GREEN}✅ Project: $PROJECT_ID${NC}\n"

#############################################################################
# Get Environment Variables from User
#############################################################################

echo -e "${YELLOW}📝 Database Configuration${NC}\n"

# DB_PASSWORD
echo -e "${BLUE}Enter your Supabase database password:${NC}"
echo -e "   (Find it in: Supabase Dashboard → Settings → Database → Database Password)"
read -s DB_PASSWORD
echo ""

if [ -z "$DB_PASSWORD" ]; then
    echo -e "${RED}❌ Database password cannot be empty!${NC}"
    exit 1
fi

# JWT_SECRET
echo -e "\n${BLUE}Generating JWT secret...${NC}"
JWT_SECRET=$(openssl rand -base64 32)
echo -e "${GREEN}✅ JWT secret generated${NC}\n"

# Confirm
echo -e "${YELLOW}📋 Deployment Configuration:${NC}"
echo "  DB_HOST: db.lqmocewqozpyzsknocrc.supabase.co"
echo "  DB_PORT: 5432"
echo "  DB_USER: postgres"
echo "  DB_PASSWORD: ********"
echo "  DB_NAME: postgres"
echo "  JWT_SECRET: ********"
echo "  ENVIRONMENT: production"
echo ""

read -p "Deploy with these settings? (y/N): " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${YELLOW}Deployment cancelled${NC}"
    exit 0
fi

#############################################################################
# Deploy
#############################################################################

echo -e "\n${BLUE}🚀 Starting deployment...${NC}\n"
echo -e "${YELLOW}This will take 3-5 minutes...${NC}\n"

gcloud builds submit \
    --config cloudbuild.yaml \
    --substitutions="_DB_PASSWORD=${DB_PASSWORD},_JWT_SECRET=${JWT_SECRET}"

#############################################################################
# Get Service URL
#############################################################################

echo -e "\n${BLUE}================================================${NC}"
echo -e "${BLUE}Deployment Complete!${NC}"
echo -e "${BLUE}================================================${NC}\n"

SERVICE_URL=$(gcloud run services describe cpls-be \
    --region=asia-southeast1 \
    --format="value(status.url)" 2>/dev/null)

if [ -n "$SERVICE_URL" ]; then
    echo -e "${GREEN}📍 Service URL:${NC}  $SERVICE_URL"
    echo -e "${GREEN}🔧 Admin UI:${NC}    $SERVICE_URL/admin"
    echo -e "${GREEN}🏥 Health:${NC}      $SERVICE_URL/health"
    echo -e "${GREEN}📡 API:${NC}         $SERVICE_URL/api/v1/"
    echo ""

    # Test health endpoint
    echo -e "${BLUE}Testing health endpoint...${NC}"
    sleep 3

    if curl -s "${SERVICE_URL}/health" | grep -q "ok"; then
        echo -e "${GREEN}✅ Service is healthy!${NC}\n"
    else
        echo -e "${YELLOW}⚠️  Service deployed but health check failed${NC}"
        echo -e "Check logs: gcloud run services logs tail cpls-be --region=asia-southeast1\n"
    fi
else
    echo -e "${YELLOW}⚠️  Could not retrieve service URL${NC}"
fi

echo -e "${GREEN}🎉 All done!${NC}\n"
