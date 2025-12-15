#!/bin/bash

###############################################################################
# Pre-Deployment Verification Script
# Kiểm tra xem đã setup đủ để deploy Google Cloud Run chưa
###############################################################################

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

ERRORS=0
WARNINGS=0

print_header() {
    echo -e "\n${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
}

check_ok() {
    echo -e "${GREEN}✅ $1${NC}"
}

check_error() {
    echo -e "${RED}❌ $1${NC}"
    ERRORS=$((ERRORS + 1))
}

check_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
    WARNINGS=$((WARNINGS + 1))
}

check_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

###############################################################################
# 1. Check gcloud CLI
###############################################################################

print_header "1. Kiểm tra gcloud CLI"

if command -v gcloud &> /dev/null; then
    GCLOUD_VERSION=$(gcloud version --format="value(core)" 2>/dev/null)
    check_ok "gcloud CLI installed (version: $GCLOUD_VERSION)"
else
    check_error "gcloud CLI NOT installed"
    echo "   Install: https://cloud.google.com/sdk/docs/install"
    exit 1
fi

# Check authentication
if gcloud auth list --filter=status:ACTIVE --format="value(account)" &> /dev/null; then
    ACCOUNT=$(gcloud auth list --filter=status:ACTIVE --format="value(account)" 2>/dev/null | head -1)
    if [ -n "$ACCOUNT" ]; then
        check_ok "Authenticated as: $ACCOUNT"
    else
        check_error "Not authenticated. Run: gcloud auth login"
    fi
else
    check_error "Authentication check failed"
fi

###############################################################################
# 2. Check Google Cloud Project
###############################################################################

print_header "2. Kiểm tra Google Cloud Project"

PROJECT_ID=$(gcloud config get-value project 2>/dev/null)

if [ -z "$PROJECT_ID" ] || [ "$PROJECT_ID" == "(unset)" ]; then
    check_error "No project configured"
    echo "   Set project: gcloud config set project YOUR_PROJECT_ID"
else
    check_ok "Project configured: $PROJECT_ID"

    # Check if project exists
    if gcloud projects describe "$PROJECT_ID" &>/dev/null; then
        check_ok "Project exists and accessible"
    else
        check_error "Project '$PROJECT_ID' not found or not accessible"
    fi
fi

# Check region
REGION=$(gcloud config get-value compute/region 2>/dev/null)
if [ -z "$REGION" ] || [ "$REGION" == "(unset)" ]; then
    check_warning "No default region set (will use asia-southeast1)"
    echo "   Recommended: gcloud config set compute/region asia-southeast1"
else
    check_ok "Region configured: $REGION"
fi

###############################################################################
# 3. Check Required APIs
###############################################################################

print_header "3. Kiểm tra Required APIs"

if [ -n "$PROJECT_ID" ] && [ "$PROJECT_ID" != "(unset)" ]; then
    APIS_TO_CHECK=(
        "run.googleapis.com:Cloud Run API"
        "cloudbuild.googleapis.com:Cloud Build API"
        "containerregistry.googleapis.com:Container Registry API"
    )

    for API_INFO in "${APIS_TO_CHECK[@]}"; do
        API="${API_INFO%%:*}"
        NAME="${API_INFO##*:}"

        if gcloud services list --enabled --filter="name:$API" --format="value(name)" 2>/dev/null | grep -q "$API"; then
            check_ok "$NAME enabled"
        else
            check_error "$NAME NOT enabled"
            echo "   Enable: gcloud services enable $API"
        fi
    done
else
    check_warning "Skipping API check (no project configured)"
fi

###############################################################################
# 4. Check Code Files
###############################################################################

print_header "4. Kiểm tra Code Files"

# Check if in correct directory
if [ -f "go.mod" ]; then
    check_ok "In Go project directory"
else
    check_error "go.mod not found - are you in the project directory?"
    exit 1
fi

# Check go.mod format
if [ -f "go.mod" ]; then
    GO_VERSION=$(grep "^go " go.mod | awk '{print $2}')
    if [ "$GO_VERSION" == "1.23" ]; then
        check_ok "go.mod version correct: go $GO_VERSION"
    else
        check_error "go.mod version incorrect: go $GO_VERSION (should be 1.23)"
        echo "   Fix: sed -i 's/^go .*/go 1.23/' go.mod"
    fi

    if grep -q "^toolchain" go.mod; then
        check_error "go.mod has toolchain directive (should be removed)"
        echo "   Fix: sed -i '/^toolchain/d' go.mod"
    else
        check_ok "go.mod has no toolchain directive"
    fi
fi

# Check Dockerfile
if [ -f "Dockerfile" ]; then
    check_ok "Dockerfile exists"

    if grep -q "FROM golang:1.23-alpine" Dockerfile; then
        check_ok "Dockerfile uses golang:1.23-alpine"
    else
        check_warning "Dockerfile may not use golang:1.23-alpine"
    fi

    if grep -q "ENV GOTOOLCHAIN=local" Dockerfile; then
        check_ok "Dockerfile has GOTOOLCHAIN=local"
    else
        check_warning "Dockerfile missing GOTOOLCHAIN=local"
    fi
else
    check_error "Dockerfile not found"
fi

# Check cloudbuild.yaml
if [ -f "cloudbuild.yaml" ]; then
    check_ok "cloudbuild.yaml exists"
else
    check_error "cloudbuild.yaml not found"
fi

# Check branch (informational only)
CURRENT_BRANCH=$(git branch --show-current 2>/dev/null)
if [ -n "$CURRENT_BRANCH" ]; then
    check_ok "On branch: $CURRENT_BRANCH"
fi

###############################################################################
# 5. Check Environment Variables Template
###############################################################################

print_header "5. Kiểm tra Environment Configuration"

if [ -f ".env.production.example" ]; then
    check_ok ".env.production.example exists (template)"
else
    check_warning ".env.production.example not found"
fi

if [ -f ".env.production" ]; then
    check_ok ".env.production exists"

    # Check if it has required vars
    REQUIRED_VARS=("DB_HOST" "DB_PASSWORD" "JWT_SECRET")
    for VAR in "${REQUIRED_VARS[@]}"; do
        if grep -q "^$VAR=" .env.production && ! grep -q "^$VAR=your-" .env.production; then
            check_ok "$VAR is set in .env.production"
        else
            check_error "$VAR not set or using placeholder in .env.production"
        fi
    done
else
    check_warning ".env.production not found (env vars will need to be set manually)"
    echo "   Create: cp .env.production.example .env.production"
fi

###############################################################################
# 6. Database Check
###############################################################################

print_header "6. Database Configuration"

if [ -f ".env.production" ]; then
    DB_HOST=$(grep "^DB_HOST=" .env.production 2>/dev/null | cut -d'=' -f2)

    if [ -n "$DB_HOST" ] && [ "$DB_HOST" != "your-db-host" ]; then
        check_ok "DB_HOST configured: $DB_HOST"

        if [[ "$DB_HOST" == *"supabase.co"* ]]; then
            check_info "Using Supabase (free tier)"
        elif [[ "$DB_HOST" == *"cloudsql"* ]]; then
            check_info "Using Cloud SQL"
        fi
    else
        check_error "DB_HOST not configured"
    fi
else
    check_warning "Cannot check database config (.env.production missing)"
    echo "   You'll need to configure database before deployment"
fi

###############################################################################
# 7. IAM Permissions Check
###############################################################################

print_header "7. IAM Permissions"

if [ -n "$PROJECT_ID" ] && [ "$PROJECT_ID" != "(unset)" ]; then
    PROJECT_NUMBER=$(gcloud projects describe "$PROJECT_ID" --format="value(projectNumber)" 2>/dev/null)

    if [ -n "$PROJECT_NUMBER" ]; then
        check_ok "Project number: $PROJECT_NUMBER"

        SERVICE_ACCOUNT="${PROJECT_NUMBER}@cloudbuild.gserviceaccount.com"

        # Check Cloud Run Admin role
        if gcloud projects get-iam-policy "$PROJECT_ID" --flatten="bindings[].members" \
           --filter="bindings.members:serviceAccount:${SERVICE_ACCOUNT}" \
           --format="value(bindings.role)" 2>/dev/null | grep -q "roles/run.admin"; then
            check_ok "Cloud Build has run.admin role"
        else
            check_warning "Cloud Build may not have run.admin role"
            echo "   Grant: See DEPLOYMENT_GUIDE section 6.2"
        fi
    fi
else
    check_warning "Cannot check IAM (no project configured)"
fi

###############################################################################
# Summary
###############################################################################

print_header "Tóm Tắt Kiểm Tra"

echo "Errors:   $ERRORS"
echo "Warnings: $WARNINGS"

if [ $ERRORS -eq 0 ] && [ $WARNINGS -eq 0 ]; then
    echo ""
    check_ok "ALL CHECKS PASSED! Ready to deploy! 🎉"
    echo ""
    echo "Deploy với:"
    echo "  ./deploy.sh"
    echo "hoặc:"
    echo "  gcloud builds submit --config cloudbuild.yaml"
    exit 0
elif [ $ERRORS -eq 0 ]; then
    echo ""
    check_warning "Ready to deploy with $WARNINGS warning(s)"
    echo ""
    echo "Bạn có thể deploy, nhưng nên fix warnings trước"
    exit 0
else
    echo ""
    check_error "NOT ready to deploy - $ERRORS error(s) found"
    echo ""
    echo "Fix errors trước khi deploy"
    echo "See: DEPLOYMENT_GUIDE_STEP_BY_STEP.md for details"
    exit 1
fi
