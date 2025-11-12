# 🚀 Hướng Dẫn Triển Khai Google Cloud Run - Từng Bước Chi Tiết

**Dự án**: CPLS Backend - Vietnamese Stock Trading System
**Ngày cập nhật**: 2025-11-12
**Thời gian triển khai dự kiến**: 30-45 phút

---

## 📋 Mục Lục

1. [Chuẩn Bị](#1-chuẩn-bị)
2. [Thiết Lập Google Cloud Project](#2-thiết-lập-google-cloud-project)
3. [Cài Đặt Google Cloud CLI](#3-cài-đặt-google-cloud-cli)
4. [Thiết Lập Database](#4-thiết-lập-database)
5. [Cấu Hình Môi Trường](#5-cấu-hình-môi-trường)
6. [Triển Khai Lần Đầu](#6-triển-khai-lần-đầu)
7. [Kiểm Tra Deployment](#7-kiểm-tra-deployment)
8. [Cấu Hình Nâng Cao](#8-cấu-hình-nâng-cao)
9. [Monitoring & Logs](#9-monitoring--logs)
10. [Troubleshooting](#10-troubleshooting)

---

## 1. Chuẩn Bị

### 1.1. Yêu Cầu Hệ Thống

- ✅ Máy tính có kết nối Internet
- ✅ Tài khoản Google (Gmail)
- ✅ Thẻ tín dụng/ghi nợ (để verify Google Cloud - có $300 free credit)
- ✅ Git đã cài đặt
- ✅ Code đã push lên branch `claude/analyze-optimize-code-011CV3EkqVvhUeTi6Z8Ap2gn`

### 1.2. Kiểm Tra Code Sẵn Sàng

```bash
# Clone repository (nếu chưa có)
git clone https://github.com/datvt88/CPLS-BE.git
cd CPLS-BE

# Checkout đúng branch
git checkout claude/analyze-optimize-code-011CV3EkqVvhUeTi6Z8Ap2gn

# Pull latest changes
git pull origin claude/analyze-optimize-code-011CV3EkqVvhUeTi6Z8Ap2gn

# Verify files quan trọng
ls -la | grep -E "Dockerfile|cloudbuild.yaml|go.mod"
```

**Kết quả mong đợi**:
```
-rw-r--r-- 1 user user  xxx  Dockerfile
-rw-r--r-- 1 user user  xxx  cloudbuild.yaml
-rw-r--r-- 1 user user  xxx  go.mod
```

### 1.3. Verify go.mod Format

```bash
head -10 go.mod
```

**Phải thấy**:
```
module go_backend_project

go 1.23         # ← Đúng format (KHÔNG có .0)

require (
  ...
)
```

**KHÔNG được có**: `toolchain go1.24.7`

---

## 2. Thiết Lập Google Cloud Project

### 2.1. Tạo Google Cloud Account

1. **Truy cập**: https://console.cloud.google.com
2. **Đăng nhập** bằng tài khoản Google
3. **Nếu lần đầu**:
   - Click "Try for Free" / "Dùng thử miễn phí"
   - Chọn quốc gia: **Vietnam**
   - Nhập thông tin thẻ (sẽ không charge nếu trong free tier)
   - Nhận **$300 free credit** (valid 90 ngày)

### 2.2. Tạo Project Mới

**Bước 1**: Click vào dropdown Project ở góc trên bên trái

**Bước 2**: Click "NEW PROJECT" / "DỰ ÁN MỚI"

**Bước 3**: Điền thông tin:
- **Project name**: `cpls-stock-trading` (hoặc tên bạn muốn)
- **Project ID**: Sẽ auto-generate, ví dụ: `cpls-stock-trading-123456`
- **Location**: No organization (để mặc định)

**Bước 4**: Click "CREATE" / "TẠO"

⏱️ **Đợi 10-20 giây** để Google tạo project

**Bước 5**: Click "SELECT PROJECT" khi được hỏi

### 2.3. Enable Required APIs

**Bước 1**: Vào menu ☰ → **APIs & Services** → **Library**

**Bước 2**: Enable các APIs sau (tìm kiếm và click ENABLE):

**API 1: Cloud Run API**
```
Tìm kiếm: "Cloud Run API"
Click: ENABLE
Đợi: 5-10 giây
```

**API 2: Cloud Build API**
```
Tìm kiếm: "Cloud Build API"
Click: ENABLE
Đợi: 5-10 giây
```

**API 3: Container Registry API**
```
Tìm kiếm: "Container Registry API"
Click: ENABLE
Đợi: 5-10 giây
```

**API 4: Artifact Registry API** (recommended)
```
Tìm kiếm: "Artifact Registry API"
Click: ENABLE
Đợi: 5-10 giây
```

✅ **Verify**: Vào **APIs & Services** → **Enabled APIs** → phải thấy 4 APIs trên

### 2.4. Ghi Chú Project ID

```bash
# Lưu Project ID (sẽ dùng nhiều lần)
# Ví dụ: cpls-stock-trading-123456

# Copy từ Google Cloud Console:
# - Góc trên bên trái, bên cạnh logo Google Cloud
# - Hoặc vào Dashboard sẽ thấy "Project ID: xxx"
```

---

## 3. Cài Đặt Google Cloud CLI

### 3.1. Cài Đặt gcloud CLI

**Trên Linux**:
```bash
# Download
curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-linux-x86_64.tar.gz

# Extract
tar -xf google-cloud-cli-linux-x86_64.tar.gz

# Install
./google-cloud-sdk/install.sh

# Add to PATH (thêm vào ~/.bashrc hoặc ~/.zshrc)
echo 'source ~/google-cloud-sdk/path.bash.inc' >> ~/.bashrc
echo 'source ~/google-cloud-sdk/completion.bash.inc' >> ~/.bashrc
source ~/.bashrc
```

**Trên macOS**:
```bash
# Sử dụng Homebrew
brew install --cask google-cloud-sdk

# Hoặc download manual từ:
# https://cloud.google.com/sdk/docs/install
```

**Trên Windows**:
```powershell
# Download installer từ:
# https://cloud.google.com/sdk/docs/install

# Chạy GoogleCloudSDKInstaller.exe
# Follow wizard
```

### 3.2. Khởi Tạo gcloud

```bash
# Initialize gcloud
gcloud init
```

**Interactive prompts**:

**1. Chọn account**:
```
Pick cloud project to use:
[1] cpls-stock-trading-123456
[2] Create a new project
```
→ Nhập số tương ứng với project bạn tạo ở bước 2.2

**2. Chọn default region**:
```
Please enter your numeric choice:
[1] asia-southeast1 (Singapore)
[2] asia-east1 (Taiwan)
[3] asia-northeast1 (Tokyo)
...
```
→ Nhập **1** (Singapore - gần Việt Nam nhất)

**3. Verify cấu hình**:
```bash
gcloud config list
```

**Kết quả mong đợi**:
```
[core]
account = your-email@gmail.com
project = cpls-stock-trading-123456

[compute]
region = asia-southeast1
zone = asia-southeast1-a
```

### 3.3. Authenticate Docker

```bash
# Configure Docker để push images lên Google Container Registry
gcloud auth configure-docker
```

**Output mong đợi**: `Docker configuration file updated`

---

## 4. Thiết Lập Database

### Option A: Sử Dụng Supabase (Recommended - Miễn Phí)

**Ưu điểm**:
- ✅ Free tier generous (500MB database, 2GB bandwidth)
- ✅ Dễ setup
- ✅ PostgreSQL managed
- ✅ Có dashboard quản lý

**Bước 1**: Tạo Supabase Account

1. Truy cập: https://supabase.com
2. Click "Start your project"
3. Sign in với GitHub hoặc email

**Bước 2**: Tạo Project Mới

1. Click "New Project"
2. Điền thông tin:
   - **Name**: `cpls-stock-trading`
   - **Database Password**: Tạo password mạnh (lưu lại!)
   - **Region**: Southeast Asia (Singapore)
   - **Pricing Plan**: Free
3. Click "Create new project"
4. ⏱️ Đợi 2-3 phút để provision database

**Bước 3**: Lấy Connection String

1. Vào project vừa tạo
2. Click ⚙️ **Settings** (góc dưới bên trái)
3. Click **Database**
4. Scroll xuống "Connection string"
5. Copy **Connection string** mode **URI**

Sẽ có dạng:
```
postgresql://postgres:[YOUR-PASSWORD]@db.xxxxxxxxxxxxx.supabase.co:5432/postgres
```

**Bước 4**: Parse Connection Info

Từ connection string trên, lấy:
```
DB_HOST=db.xxxxxxxxxxxxx.supabase.co
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=[YOUR-PASSWORD]
DB_NAME=postgres
```

✅ **Lưu lại** các thông tin này - sẽ dùng ở bước 5

### Option B: Sử Dụng Cloud SQL (Paid - Tốt cho Production)

**Chi phí**: ~$10-30/tháng tùy config

**Bước 1**: Enable Cloud SQL API
```bash
gcloud services enable sqladmin.googleapis.com
```

**Bước 2**: Tạo Cloud SQL Instance
```bash
gcloud sql instances create cpls-db \
  --database-version=POSTGRES_15 \
  --tier=db-f1-micro \
  --region=asia-southeast1 \
  --root-password=YOUR_STRONG_PASSWORD
```

⏱️ **Đợi 5-10 phút**

**Bước 3**: Tạo Database
```bash
gcloud sql databases create cpls_db --instance=cpls-db
```

**Bước 4**: Lấy Connection Info
```bash
# Lấy instance connection name
gcloud sql instances describe cpls-db --format="value(connectionName)"
# Output: cpls-stock-trading-123456:asia-southeast1:cpls-db
```

**Bước 5**: Setup Cloud SQL Proxy (cho Cloud Run)
```
DB_HOST=/cloudsql/cpls-stock-trading-123456:asia-southeast1:cpls-db
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=YOUR_STRONG_PASSWORD
DB_NAME=cpls_db
```

---

## 5. Cấu Hình Môi Trường

### 5.1. Tạo File Environment Variables

```bash
# Di chuyển vào thư mục project
cd /path/to/CPLS-BE

# Tạo file .env.production (để reference, KHÔNG commit file này)
cat > .env.production << 'EOF'
# Database Configuration (từ Supabase hoặc Cloud SQL)
DB_HOST=db.xxxxxxxxxxxxx.supabase.co
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your-strong-password-here
DB_NAME=postgres

# Application Configuration
PORT=8080
ENVIRONMENT=production

# Security
JWT_SECRET=your-random-secret-key-here-at-least-32-chars

# Trading Configuration (optional)
DEFAULT_COMMISSION_RATE=0.0015
DEFAULT_TAX_RATE=0.001

# Redis (optional - nếu dùng caching)
REDIS_HOST=
REDIS_PORT=6379
EOF
```

### 5.2. Tạo JWT Secret

```bash
# Generate random JWT secret
openssl rand -base64 32

# Output example:
# aB3dE5fG7hI9jK1lM3nO5pQ7rS9tU1vW3xY5zA==

# Copy và paste vào JWT_SECRET ở .env.production
```

### 5.3. Verify Environment Variables

```bash
cat .env.production
```

**Checklist**:
- ✅ DB_HOST có giá trị đúng (Supabase hoặc Cloud SQL)
- ✅ DB_PASSWORD đã điền
- ✅ JWT_SECRET có ít nhất 32 characters
- ✅ ENVIRONMENT=production

---

## 6. Triển Khai Lần Đầu

### 6.1. Verify Project Settings

```bash
# Kiểm tra đang ở đúng project
gcloud config get-value project

# Nếu không đúng, set lại:
gcloud config set project cpls-stock-trading-123456
```

### 6.2. Grant Permissions cho Cloud Build

```bash
# Lấy Project Number
PROJECT_NUMBER=$(gcloud projects describe $(gcloud config get-value project) --format="value(projectNumber)")

echo "Project Number: $PROJECT_NUMBER"

# Grant Cloud Run Admin role cho Cloud Build service account
gcloud projects add-iam-policy-binding $(gcloud config get-value project) \
  --member="serviceAccount:${PROJECT_NUMBER}@cloudbuild.gserviceaccount.com" \
  --role="roles/run.admin"

# Grant Service Account User role
gcloud projects add-iam-policy-binding $(gcloud config get-value project) \
  --member="serviceAccount:${PROJECT_NUMBER}@cloudbuild.gserviceaccount.com" \
  --role="roles/iam.serviceAccountUser"
```

### 6.3. Review cloudbuild.yaml

```bash
cat cloudbuild.yaml
```

**Nội dung hiện tại**:
```yaml
steps:
  # Build Docker image
  - name: 'gcr.io/cloud-builders/docker'
    args: ['build', '-t', 'gcr.io/$PROJECT_ID/go-backend', '.']

  # Push to Container Registry
  - name: 'gcr.io/cloud-builders/docker'
    args: ['push', 'gcr.io/$PROJECT_ID/go-backend']

  # Deploy to Cloud Run
  - name: 'gcr.io/google.com/cloudsdktool/cloud-sdk'
    args:
      - 'run'
      - 'deploy'
      - 'go-backend'
      - '--image'
      - 'gcr.io/$PROJECT_ID/go-backend'
      - '--region'
      - 'asia-southeast1'
      - '--platform'
      - 'managed'
      - '--allow-unauthenticated'
```

✅ File này đã OK, không cần sửa

### 6.4. Update cloudbuild.yaml với Environment Variables

**Tạo enhanced version với env vars**:

```bash
cat > cloudbuild.yaml << 'EOF'
steps:
  # Build Docker image
  - name: 'gcr.io/cloud-builders/docker'
    args: ['build', '-t', 'gcr.io/$PROJECT_ID/cpls-backend', '.']

  # Push to Container Registry
  - name: 'gcr.io/cloud-builders/docker'
    args: ['push', 'gcr.io/$PROJECT_ID/cpls-backend']

  # Deploy to Cloud Run
  - name: 'gcr.io/google.com/cloudsdktool/cloud-sdk'
    entrypoint: gcloud
    args:
      - 'run'
      - 'deploy'
      - 'cpls-backend'
      - '--image=gcr.io/$PROJECT_ID/cpls-backend'
      - '--region=asia-southeast1'
      - '--platform=managed'
      - '--allow-unauthenticated'
      - '--memory=512Mi'
      - '--cpu=1'
      - '--max-instances=10'
      - '--set-env-vars=ENVIRONMENT=production'
      - '--set-env-vars=PORT=8080'

images:
  - 'gcr.io/$PROJECT_ID/cpls-backend'

options:
  machineType: 'E2_HIGHCPU_8'
  logging: CLOUD_LOGGING_ONLY
EOF
```

**⚠️ Lưu ý**: Chúng ta sẽ set DB credentials sau khi deploy lần đầu (để secure hơn)

### 6.5. Deploy Lần Đầu

```bash
# Đảm bảo đang ở thư mục gốc của project
cd /path/to/CPLS-BE

# Submit build
gcloud builds submit --config cloudbuild.yaml
```

**Quá trình deploy**:

```
Creating temporary archive of xx files in /path/to/CPLS-BE...
Uploading tarball of [.] to [gs://xxx]...
Created [https://cloudbuild.googleapis.com/v1/projects/xxx/builds/xxx].
Logs are available at [https://console.cloud.google.com/cloud-build/builds/xxx].

------------------------------------------------- REMOTE BUILD OUTPUT --------------------------------------------------
starting build "xxx"

FETCHSOURCE
Fetching storage object...
BUILD
Already have image (with digest): gcr.io/cloud-builders/docker
Sending build context to Docker daemon  xxx MB
Step 1/11 : FROM golang:1.23-alpine
1.23-alpine: Pulling from library/golang
...
Successfully built xxxxx
Successfully tagged gcr.io/cpls-stock-trading-123456/cpls-backend:latest

PUSH
The push refers to repository [gcr.io/cpls-stock-trading-123456/cpls-backend]
...
latest: digest: sha256:xxx size: 1234

DEPLOY
Deploying container to Cloud Run service [cpls-backend] in project [xxx] region [asia-southeast1]
✓ Deploying new service... Done.
  ✓ Creating Revision...
  ✓ Routing traffic...
Done.
Service [cpls-backend] revision [cpls-backend-00001-xxx] has been deployed and is serving 100 percent of traffic.
Service URL: https://cpls-backend-xxxxx-as.a.run.app
```

⏱️ **Thời gian**: 3-5 phút cho lần đầu

✅ **Thành công khi thấy**:
- `Successfully built`
- `Successfully tagged`
- `Deploying new service... Done`
- `Service URL: https://...`

**🎉 Lưu lại Service URL** - đây là URL của ứng dụng!

### 6.6. Set Database Environment Variables (Secure)

```bash
# Đọc từ file .env.production
source .env.production

# Set environment variables cho Cloud Run
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --set-env-vars="DB_HOST=${DB_HOST}" \
  --set-env-vars="DB_PORT=${DB_PORT}" \
  --set-env-vars="DB_USER=${DB_USER}" \
  --set-env-vars="DB_PASSWORD=${DB_PASSWORD}" \
  --set-env-vars="DB_NAME=${DB_NAME}" \
  --set-env-vars="JWT_SECRET=${JWT_SECRET}" \
  --set-env-vars="ENVIRONMENT=production"
```

**Hoặc set từng cái manually** (nếu không dùng .env.production):

```bash
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --update-env-vars="DB_HOST=db.xxxxx.supabase.co,DB_PORT=5432,DB_USER=postgres,DB_PASSWORD=your-password,DB_NAME=postgres,JWT_SECRET=your-secret,ENVIRONMENT=production"
```

**Verify env vars đã set**:

```bash
gcloud run services describe cpls-backend \
  --region=asia-southeast1 \
  --format="value(spec.template.spec.containers[0].env)"
```

---

## 7. Kiểm Tra Deployment

### 7.1. Get Service URL

```bash
# Lấy URL của service
SERVICE_URL=$(gcloud run services describe cpls-backend \
  --region=asia-southeast1 \
  --format="value(status.url)")

echo "Service URL: $SERVICE_URL"

# Output example: https://cpls-backend-xxxxx-as.a.run.app
```

### 7.2. Test Health Endpoint

```bash
# Test health check
curl $SERVICE_URL/health

# Expected response:
# {"status":"ok"}
```

✅ Nếu thấy `{"status":"ok"}` → Backend đã chạy!

### 7.3. Test Database Connection

```bash
# Check logs để xem database connection
gcloud run services logs read cpls-backend \
  --region=asia-southeast1 \
  --limit=50

# Tìm dòng:
# "Database connected successfully" hoặc
# "Auto-migrating database..."
```

**Nếu có lỗi database**, xem [Troubleshooting](#10-troubleshooting)

### 7.4. Test Admin UI

```bash
# Mở browser với Admin UI
echo "Admin URL: $SERVICE_URL/admin"

# Hoặc
open $SERVICE_URL/admin    # macOS
xdg-open $SERVICE_URL/admin  # Linux
start $SERVICE_URL/admin   # Windows
```

**Phải thấy**: Admin Dashboard với statistics (0 stocks, 0 strategies, etc.)

### 7.5. Test API Endpoints

```bash
# Test API health
curl $SERVICE_URL/api/v1/health

# Test stocks endpoint (sẽ trả về empty array lần đầu)
curl $SERVICE_URL/api/v1/stocks

# Expected:
# {"data":[],"total":0,"page":1,"limit":10}
```

---

## 8. Cấu Hình Nâng Cao

### 8.1. Setup Custom Domain (Optional)

**Yêu cầu**: Có domain riêng (ví dụ: cpls.yourdomain.com)

**Bước 1**: Map domain

```bash
gcloud run domain-mappings create \
  --service=cpls-backend \
  --domain=cpls.yourdomain.com \
  --region=asia-southeast1
```

**Bước 2**: Cấu hình DNS

```bash
# Cloud Run sẽ cho bạn DNS records cần thêm
# Ví dụ:
# Type: CNAME
# Name: cpls
# Value: ghs.googlehosted.com
```

Vào nhà cung cấp domain (GoDaddy, Namecheap, etc.) và thêm CNAME record

⏱️ **Đợi**: 5-60 phút để DNS propagate

### 8.2. Configure Scaling

```bash
# Set min instances (tránh cold start)
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --min-instances=1 \
  --max-instances=10

# Set concurrency (số requests/instance)
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --concurrency=80

# Set memory & CPU
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --memory=1Gi \
  --cpu=2
```

**Chi phí estimate với min-instances=1**:
- ~$5-10/tháng cho 1 instance luôn chạy
- Free tier: 180,000 vCPU-seconds/month

### 8.3. Setup Cloud Scheduler (Auto Data Update)

```bash
# Enable Cloud Scheduler API
gcloud services enable cloudscheduler.googleapis.com

# Create job để update stock data mỗi ngày
gcloud scheduler jobs create http stock-data-daily \
  --schedule="0 18 * * 1-5" \
  --uri="${SERVICE_URL}/api/v1/stocks/update" \
  --http-method=POST \
  --time-zone="Asia/Ho_Chi_Minh" \
  --location=asia-southeast1 \
  --description="Update stock data daily at 6 PM on weekdays"
```

**Schedule format** (cron):
- `0 18 * * 1-5` = 6:00 PM, Thứ 2-6 (weekdays)
- `0 9 * * *` = 9:00 AM mỗi ngày
- `*/30 * * * *` = Mỗi 30 phút

### 8.4. Setup Secret Manager (Secure Secrets)

**Recommended cho production**:

```bash
# Enable Secret Manager API
gcloud services enable secretmanager.googleapis.com

# Create secrets
echo -n "your-db-password" | gcloud secrets create db-password --data-file=-
echo -n "your-jwt-secret" | gcloud secrets create jwt-secret --data-file=-

# Grant access to Cloud Run
gcloud secrets add-iam-policy-binding db-password \
  --member="serviceAccount:${PROJECT_NUMBER}-compute@developer.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"

gcloud secrets add-iam-policy-binding jwt-secret \
  --member="serviceAccount:${PROJECT_NUMBER}-compute@developer.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"

# Update Cloud Run to use secrets
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --update-secrets=DB_PASSWORD=db-password:latest \
  --update-secrets=JWT_SECRET=jwt-secret:latest
```

### 8.5. Setup VPC Connector (Nếu dùng Cloud SQL)

```bash
# Enable VPC Access API
gcloud services enable vpcaccess.googleapis.com

# Create VPC connector
gcloud compute networks vpc-access connectors create cpls-connector \
  --region=asia-southeast1 \
  --range=10.8.0.0/28

# Update Cloud Run to use VPC
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --vpc-connector=cpls-connector \
  --vpc-egress=private-ranges-only
```

---

## 9. Monitoring & Logs

### 9.1. Xem Logs Real-time

```bash
# Stream logs real-time
gcloud run services logs tail cpls-backend \
  --region=asia-southeast1

# Xem logs với filter
gcloud run services logs read cpls-backend \
  --region=asia-southeast1 \
  --filter="severity>=ERROR" \
  --limit=100
```

### 9.2. Cloud Console Logs

1. Truy cập: https://console.cloud.google.com/run
2. Click vào service **cpls-backend**
3. Click tab **LOGS**

**Features**:
- Filter by severity (INFO, WARNING, ERROR)
- Search by text
- Time range selection
- Download logs

### 9.3. Monitoring Dashboard

```bash
# Mở monitoring dashboard
gcloud run services describe cpls-backend \
  --region=asia-southeast1 \
  --format="value(status.url)"
```

**Hoặc via Console**:
1. Cloud Run → cpls-backend
2. Tab **METRICS**

**Metrics available**:
- Request count
- Request latency (P50, P95, P99)
- Container instance count
- CPU utilization
- Memory utilization
- Billable container time

### 9.4. Setup Alerts

**Via Console**:

1. Cloud Run → cpls-backend → METRICS
2. Click "CREATE ALERT"
3. Cấu hình:
   - **Metric**: Request latency (P95)
   - **Condition**: > 1000ms
   - **Duration**: 5 minutes
   - **Notification**: Email

**Via gcloud**:

```bash
# Tạo notification channel
gcloud alpha monitoring channels create \
  --display-name="Email Alert" \
  --type=email \
  --channel-labels=email_address=your-email@gmail.com

# Tạo alert policy (cần config file)
# Xem: https://cloud.google.com/monitoring/alerts
```

---

## 10. Troubleshooting

### 10.1. Build Failed

**Error**: `go: updates to go.mod needed`

**Solution**:
```bash
# Verify go.mod format
head -10 go.mod

# Must be "go 1.23" (NOT "go 1.23.0")
# Must NOT have "toolchain" directive

# If wrong, fix:
sed -i 's/go 1.23.0/go 1.23/' go.mod
sed -i '/^toolchain/d' go.mod

# Commit and push
git add go.mod
git commit -m "Fix go.mod format for Cloud Run"
git push origin claude/analyze-optimize-code-011CV3EkqVvhUeTi6Z8Ap2gn

# Retry build
gcloud builds submit --config cloudbuild.yaml
```

### 10.2. Service Failed to Start

**Error**: Service shows "Revision failed"

**Check logs**:
```bash
gcloud run services logs read cpls-backend \
  --region=asia-southeast1 \
  --limit=100
```

**Common issues**:

**Issue 1: Database connection failed**
```
Error: failed to connect to database
```

**Solution**:
```bash
# Verify env vars are set
gcloud run services describe cpls-backend \
  --region=asia-southeast1 \
  --format="value(spec.template.spec.containers[0].env)"

# Re-set if missing
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --update-env-vars="DB_HOST=xxx,DB_PASSWORD=xxx,..."
```

**Issue 2: Port binding failed**
```
Error: listen tcp :8080: bind: address already in use
```

**Solution**: Đã được handle trong code (config.go đọc PORT từ env)
```bash
# Verify PORT env var
gcloud run services describe cpls-backend \
  --region=asia-southeast1 \
  --format="value(spec.template.spec.containers[0].env)" | grep PORT
```

### 10.3. Database Migration Issues

**Error**: Tables không được tạo

**Solution**:
```bash
# Check logs cho migration messages
gcloud run services logs read cpls-backend \
  --region=asia-southeast1 | grep -i "migrat"

# Nếu không thấy migration logs, có thể cần trigger manually
# Connect to database và check tables:
```

**Via Supabase Dashboard**:
1. Supabase → Project → Table Editor
2. Phải thấy tables: stocks, stock_prices, trading_strategies, etc.

**Via psql**:
```bash
# Connect to Supabase
psql "postgresql://postgres:[PASSWORD]@db.xxxxx.supabase.co:5432/postgres"

# List tables
\dt

# Should see:
# stocks, stock_prices, technical_indicators, market_indices
# trading_strategies, trades, portfolios, backtests, backtest_trades, signals
```

### 10.4. Deployment Timeout

**Error**: `ERROR: (gcloud.run.deploy) Revision creation timeout`

**Solution**:
```bash
# Increase timeout
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --timeout=300

# Increase memory if OOM
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --memory=1Gi
```

### 10.5. Permission Denied Errors

**Error**: `Permission denied` during build/deploy

**Solution**:
```bash
# Re-grant permissions
PROJECT_NUMBER=$(gcloud projects describe $(gcloud config get-value project) --format="value(projectNumber)")

gcloud projects add-iam-policy-binding $(gcloud config get-value project) \
  --member="serviceAccount:${PROJECT_NUMBER}@cloudbuild.gserviceaccount.com" \
  --role="roles/run.admin"

gcloud projects add-iam-policy-binding $(gcloud config get-value project) \
  --member="serviceAccount:${PROJECT_NUMBER}@cloudbuild.gserviceaccount.com" \
  --role="roles/iam.serviceAccountUser"
```

### 10.6. Cost Unexpected

**Check billing**:
```bash
# View current billing
gcloud beta billing accounts list

# View project costs
gcloud billing projects describe $(gcloud config get-value project)
```

**Via Console**:
- Billing → Reports → Chọn project

**Reduce costs**:
```bash
# Set max instances
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --max-instances=3

# Remove min instances (allow scale to zero)
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --no-min-instances

# Reduce memory
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --memory=512Mi
```

---

## 11. Update Deployment (Sau Khi Code Thay Đổi)

### 11.1. Quick Update

```bash
# Pull latest code
git pull origin claude/analyze-optimize-code-011CV3EkqVvhUeTi6Z8Ap2gn

# Deploy lại
gcloud builds submit --config cloudbuild.yaml

# Cloud Run tự động:
# - Build image mới
# - Deploy revision mới
# - Route 100% traffic to new revision
# - Keep old revision as backup
```

### 11.2. Rollback Nếu Có Vấn Đề

```bash
# List revisions
gcloud run revisions list \
  --service=cpls-backend \
  --region=asia-southeast1

# Rollback to previous revision
gcloud run services update-traffic cpls-backend \
  --region=asia-southeast1 \
  --to-revisions=cpls-backend-00001-xxx=100
```

---

## 12. Production Checklist

### Pre-Production

- [ ] Database có backup plan
- [ ] Environment variables đã set đầy đủ
- [ ] JWT_SECRET đủ mạnh (32+ chars random)
- [ ] Database password đủ mạnh
- [ ] Alerts đã setup
- [ ] Monitoring dashboard đã check
- [ ] Logs có thể access được

### Security

- [ ] Sử dụng Secret Manager cho sensitive data
- [ ] Enable authentication cho Admin UI
- [ ] API rate limiting đã setup
- [ ] CORS configuration kiểm tra
- [ ] Database SSL connection enabled (Supabase default có)
- [ ] Regular security updates

### Performance

- [ ] Min instances set phù hợp (nếu cần low latency)
- [ ] Memory/CPU sizing phù hợp với load
- [ ] Database indexes đã optimize
- [ ] Monitoring metrics trong ngưỡng OK

### Compliance

- [ ] Data residency requirements met (Singapore region)
- [ ] Logging compliant với retention policies
- [ ] Backup strategy documented

---

## 13. Useful Commands Reference

### Quick Commands

```bash
# Deploy
gcloud builds submit --config cloudbuild.yaml

# View service URL
gcloud run services describe cpls-backend --region=asia-southeast1 --format="value(status.url)"

# View logs
gcloud run services logs tail cpls-backend --region=asia-southeast1

# Update env vars
gcloud run services update cpls-backend --region=asia-southeast1 --update-env-vars="KEY=VALUE"

# Scale
gcloud run services update cpls-backend --region=asia-southeast1 --min-instances=1 --max-instances=10

# Delete service (cleanup)
gcloud run services delete cpls-backend --region=asia-southeast1
```

### Debugging Commands

```bash
# Get full service description
gcloud run services describe cpls-backend --region=asia-southeast1

# Get revision details
gcloud run revisions describe REVISION_NAME --region=asia-southeast1

# Test locally (before deploy)
docker build -t cpls-test .
docker run -p 8080:8080 --env-file .env.production cpls-test

# Shell into running container (for debugging)
gcloud run services proxy cpls-backend --region=asia-southeast1
```

---

## 14. Cost Estimation

### Free Tier (Generous)

- **Requests**: 2M/month
- **CPU time**: 360,000 vCPU-seconds/month
- **Memory**: 180,000 GiB-seconds/month
- **Bandwidth**: 1 GB/month

### Estimated Monthly Cost

**Scenario 1: Low traffic (within free tier)**
- Requests: 100K/month
- Average response: 200ms
- Cost: **$0/month** ✅

**Scenario 2: Medium traffic**
- Requests: 5M/month
- Average response: 200ms
- Min instances: 0 (scale to zero)
- Cost: **~$5-10/month**

**Scenario 3: High traffic with min instances**
- Requests: 10M/month
- Average response: 200ms
- Min instances: 1 (always on)
- Memory: 512Mi
- Cost: **~$15-25/month**

**Database (Supabase Free)**:
- Up to 500MB: **$0/month**
- Over 500MB: Upgrade to Pro ($25/month)

**Total for development/small production**: **$0-35/month**

---

## 15. Next Steps

### Immediate (Ngay sau deploy)

1. ✅ Test all API endpoints
2. ✅ Initialize stock data via Admin UI
3. ✅ Create first trading strategy
4. ✅ Run test backtest
5. ✅ Monitor logs for 24h

### Short-term (Tuần đầu tiên)

1. Setup custom domain (nếu có)
2. Enable authentication cho Admin UI
3. Setup daily data update scheduler
4. Configure alerting
5. Document API cho team

### Long-term (Sau 1 tháng)

1. Connect real Vietnamese stock APIs (SSI, VNDirect, TCBS)
2. Implement advanced trading strategies
3. Add websocket for real-time updates
4. Mobile app integration
5. Scale based on actual usage patterns

---

## 🎉 Kết Luận

Bạn đã hoàn thành deployment CPLS Backend lên Google Cloud Run!

**Service URL**: `https://cpls-backend-xxxxx-as.a.run.app`

**Admin UI**: `https://cpls-backend-xxxxx-as.a.run.app/admin`

**API Docs**: See `README.md` for complete API documentation

**Support**:
- Google Cloud Console: https://console.cloud.google.com
- Cloud Run Docs: https://cloud.google.com/run/docs
- Supabase Docs: https://supabase.com/docs

---

**Happy Trading! 📈🚀**

*Tài liệu này được tạo ngày 2025-11-12*
*Version: 1.0*
*Branch: claude/analyze-optimize-code-011CV3EkqVvhUeTi6Z8Ap2gn*
