#!/bin/bash

#############################################################################
# CPLS Backend - Automated Google Cloud Run Deployment Script
#############################################################################
# Usage: ./deploy.sh [options]
# Options:
#   --project PROJECT_ID    Set Google Cloud project ID
#   --region REGION         Set deployment region (default: asia-southeast1)
#   --service SERVICE_NAME  Set service name (default: cpls-backend)
#   --help                  Show this help message
#############################################################################

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
REGION="asia-southeast1"
SERVICE_NAME="cpls-backend"
PROJECT_ID=""

#############################################################################
# Helper Functions
#############################################################################

print_header() {
    echo -e "\n${BLUE}================================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}================================================${NC}\n"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

#############################################################################
# Parse Arguments
#############################################################################

while [[ $# -gt 0 ]]; do
    case $1 in
        --project)
            PROJECT_ID="$2"
            shift 2
            ;;
        --region)
            REGION="$2"
            shift 2
            ;;
        --service)
            SERVICE_NAME="$2"
            shift 2
            ;;
        --help)
            head -n 11 "$0" | tail -n 8
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

#############################################################################
# Pre-flight Checks
#############################################################################

print_header "Pre-flight Checks"

# Check if gcloud is installed
if ! command -v gcloud &> /dev/null; then
    print_error "gcloud CLI not found!"
    echo "Please install: https://cloud.google.com/sdk/docs/install"
    exit 1
fi
print_success "gcloud CLI installed"

# Check if git is installed
if ! command -v git &> /dev/null; then
    print_error "git not found!"
    exit 1
fi
print_success "git installed"

# Check if in correct directory
if [ ! -f "go.mod" ] || [ ! -f "Dockerfile" ]; then
    print_error "Not in CPLS-BE directory!"
    echo "Please run this script from the project root"
    exit 1
fi
print_success "In correct directory"

# Check branch (informational only)
CURRENT_BRANCH=$(git branch --show-current)
if [ -n "$CURRENT_BRANCH" ]; then
    print_success "On branch: $CURRENT_BRANCH"
else
    print_warning "Not on any branch (detached HEAD)"
fi

# Verify go.mod format
GO_VERSION=$(head -10 go.mod | grep "^go " | awk '{print $2}')
if [ "$GO_VERSION" == "1.23" ]; then
    print_success "go.mod format correct: go $GO_VERSION"
else
    print_error "go.mod format incorrect: go $GO_VERSION"
    print_info "Fixing go.mod format..."
    sed -i 's/^go 1\.23\.0$/go 1.23/' go.mod
    sed -i '/^toolchain/d' go.mod
    print_success "go.mod fixed"
fi

# Check for toolchain directive
if grep -q "^toolchain" go.mod; then
    print_warning "Toolchain directive found in go.mod, removing..."
    sed -i '/^toolchain/d' go.mod
    print_success "Toolchain directive removed"
fi

#############################################################################
# Get/Set Project
#############################################################################

print_header "Google Cloud Project Configuration"

if [ -z "$PROJECT_ID" ]; then
    # Try to get from gcloud config
    PROJECT_ID=$(gcloud config get-value project 2>/dev/null)

    if [ -z "$PROJECT_ID" ] || [ "$PROJECT_ID" == "(unset)" ]; then
        print_error "No project configured!"
        echo "Please run: gcloud init"
        echo "Or use: ./deploy.sh --project YOUR_PROJECT_ID"
        exit 1
    fi
fi

gcloud config set project "$PROJECT_ID" 2>/dev/null
print_success "Using project: $PROJECT_ID"
print_info "Region: $REGION"
print_info "Service name: $SERVICE_NAME"

#############################################################################
# Enable APIs
#############################################################################

print_header "Enabling Required APIs"

APIS=(
    "run.googleapis.com"
    "cloudbuild.googleapis.com"
    "containerregistry.googleapis.com"
)

for API in "${APIS[@]}"; do
    print_info "Enabling $API..."
    gcloud services enable "$API" --project="$PROJECT_ID" 2>/dev/null || true
done

print_success "APIs enabled"

#############################################################################
# Grant Permissions
#############################################################################

print_header "Setting up Permissions"

PROJECT_NUMBER=$(gcloud projects describe "$PROJECT_ID" --format="value(projectNumber)")
print_info "Project number: $PROJECT_NUMBER"

print_info "Granting Cloud Run Admin role..."
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
    --member="serviceAccount:${PROJECT_NUMBER}@cloudbuild.gserviceaccount.com" \
    --role="roles/run.admin" \
    --condition=None \
    2>/dev/null || true

print_info "Granting Service Account User role..."
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
    --member="serviceAccount:${PROJECT_NUMBER}@cloudbuild.gserviceaccount.com" \
    --role="roles/iam.serviceAccountUser" \
    --condition=None \
    2>/dev/null || true

print_success "Permissions configured"

#############################################################################
# Build & Deploy
#############################################################################

print_header "Building and Deploying"

print_info "Starting Cloud Build..."
print_warning "This will take 3-5 minutes..."

# Submit build
if gcloud builds submit --config cloudbuild.yaml --project="$PROJECT_ID"; then
    print_success "Build and deployment successful!"
else
    print_error "Build failed!"
    echo ""
    echo "To view logs:"
    echo "  gcloud builds log --region=$REGION"
    echo ""
    echo "Common issues:"
    echo "  1. Check go.mod format (should be 'go 1.23')"
    echo "  2. Verify Dockerfile exists"
    echo "  3. Check cloudbuild.yaml syntax"
    exit 1
fi

#############################################################################
# Get Service URL
#############################################################################

print_header "Deployment Summary"

SERVICE_URL=$(gcloud run services describe "$SERVICE_NAME" \
    --region="$REGION" \
    --format="value(status.url)" \
    --project="$PROJECT_ID" 2>/dev/null)

if [ -n "$SERVICE_URL" ]; then
    print_success "Service deployed successfully!"
    echo ""
    echo -e "${GREEN}📍 Service URL:${NC} $SERVICE_URL"
    echo -e "${GREEN}🔧 Admin UI:${NC}   $SERVICE_URL/admin"
    echo -e "${GREEN}🏥 Health:${NC}     $SERVICE_URL/health"
    echo -e "${GREEN}📡 API:${NC}        $SERVICE_URL/api/v1/"
    echo ""
else
    print_warning "Could not retrieve service URL"
    echo "Get it manually with:"
    echo "  gcloud run services describe $SERVICE_NAME --region=$REGION --format='value(status.url)'"
fi

#############################################################################
# Environment Variables Reminder
#############################################################################

print_header "Next Steps"

print_info "Set environment variables for database connection:"
echo ""
echo "gcloud run services update $SERVICE_NAME \\"
echo "  --region=$REGION \\"
echo "  --update-env-vars=\"DB_HOST=db.xxxxx.supabase.co,\\"
echo "DB_PORT=5432,\\"
echo "DB_USER=postgres,\\"
echo "DB_PASSWORD=YOUR_PASSWORD,\\"
echo "DB_NAME=postgres,\\"
echo "JWT_SECRET=\$(openssl rand -base64 32),\\"
echo "ENVIRONMENT=production\""
echo ""

print_info "Test the deployment:"
echo ""
echo "curl $SERVICE_URL/health"
echo ""

print_info "View logs:"
echo ""
echo "gcloud run services logs tail $SERVICE_NAME --region=$REGION"
echo ""

print_success "Deployment complete!"

#############################################################################
# Test Health Endpoint
#############################################################################

if [ -n "$SERVICE_URL" ]; then
    print_header "Health Check"

    print_info "Testing health endpoint..."
    sleep 3  # Give service a moment to start

    if curl -s "${SERVICE_URL}/health" | grep -q "ok"; then
        print_success "Health check passed!"
    else
        print_warning "Health check failed or service not ready yet"
        print_info "You may need to set environment variables first"
    fi
fi

echo ""
print_success "All done! 🎉"
echo ""
