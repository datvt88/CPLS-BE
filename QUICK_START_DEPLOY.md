# ⚡ Quick Start - Deploy lên Google Cloud Run trong 15 phút

**Prerequisite**: Đã có tài khoản Google Cloud và đã enable billing

---

## Bước 1: Cài đặt gcloud CLI (5 phút)

```bash
# Linux
curl https://sdk.cloud.google.com | bash
exec -l $SHELL

# macOS
brew install --cask google-cloud-sdk

# Khởi tạo
gcloud init
# Chọn: [1] Log in with a new account
# Chọn project hoặc tạo mới
# Chọn region: asia-southeast1
```

---

## Bước 2: Clone & Checkout Code (1 phút)

```bash
git clone https://github.com/datvt88/CPLS-BE.git
cd CPLS-BE
git checkout claude/analyze-optimize-code-011CV3EkqVvhUeTi6Z8Ap2gn
```

---

## Bước 3: Setup Database - Supabase (3 phút)

1. **Truy cập**: https://supabase.com
2. **Sign up** và click "New Project"
3. **Config**:
   - Name: `cpls-trading`
   - Password: Tạo password mạnh (lưu lại!)
   - Region: Southeast Asia (Singapore)
   - Plan: Free
4. **Đợi** 2 phút database provision
5. **Copy connection info**:
   - Settings → Database → Connection string
   - Lưu lại: host, password

---

## Bước 4: Enable Google Cloud APIs (2 phút)

```bash
# Enable required APIs
gcloud services enable run.googleapis.com
gcloud services enable cloudbuild.googleapis.com
gcloud services enable containerregistry.googleapis.com

# Grant permissions
PROJECT_NUMBER=$(gcloud projects describe $(gcloud config get-value project) --format="value(projectNumber)")

gcloud projects add-iam-policy-binding $(gcloud config get-value project) \
  --member="serviceAccount:${PROJECT_NUMBER}@cloudbuild.gserviceaccount.com" \
  --role="roles/run.admin"

gcloud projects add-iam-policy-binding $(gcloud config get-value project) \
  --member="serviceAccount:${PROJECT_NUMBER}@cloudbuild.gserviceaccount.com" \
  --role="roles/iam.serviceAccountUser"
```

---

## Bước 5: Deploy (2 phút)

```bash
# Từ thư mục CPLS-BE
gcloud builds submit --config cloudbuild.yaml
```

⏱️ Đợi 3-5 phút...

✅ Khi thấy: `Service URL: https://cpls-backend-xxxxx-as.a.run.app`

**Lưu lại URL này!**

---

## Bước 6: Set Environment Variables (2 phút)

```bash
# Thay YOUR_xxx bằng giá trị thực từ Supabase
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --update-env-vars="DB_HOST=db.xxxxxxxxxxxxx.supabase.co,DB_PORT=5432,DB_USER=postgres,DB_PASSWORD=YOUR_DB_PASSWORD,DB_NAME=postgres,JWT_SECRET=$(openssl rand -base64 32),ENVIRONMENT=production"
```

---

## Bước 7: Verify (1 phút)

```bash
# Get service URL
SERVICE_URL=$(gcloud run services describe cpls-backend --region=asia-southeast1 --format="value(status.url)")

# Test health
curl $SERVICE_URL/health
# Expected: {"status":"ok"}

# Mở Admin UI trong browser
echo "Admin URL: $SERVICE_URL/admin"
```

---

## 🎉 Done!

**Your app is live at**: `https://cpls-backend-xxxxx-as.a.run.app`

**Admin UI**: `https://cpls-backend-xxxxx-as.a.run.app/admin`

**API**: `https://cpls-backend-xxxxx-as.a.run.app/api/v1/`

---

## Next Steps

1. **Initialize data**: Vào Admin UI → Click "Initialize Stock Data"
2. **Create strategy**: Admin → Strategies → Create Strategy
3. **Run backtest**: Admin → Backtests → Run Backtest
4. **Start bot**: Admin → Trading Bot → Start Bot

---

## Troubleshooting

**Nếu service không start**:
```bash
# Check logs
gcloud run services logs read cpls-backend --region=asia-southeast1 --limit=50

# Common issue: Database connection
# → Verify DB_HOST, DB_PASSWORD trong env vars
```

**Nếu build failed**:
```bash
# Verify go.mod format
head -10 go.mod
# Must show: "go 1.23" (NOT "go 1.23.0")
```

---

## Update Code

```bash
# Pull latest changes
git pull origin claude/analyze-optimize-code-011CV3EkqVvhUeTi6Z8Ap2gn

# Re-deploy
gcloud builds submit --config cloudbuild.yaml
```

---

## Cost

**Free tier**: 2M requests/month, 360K vCPU-seconds/month

**Expected cost** (low-medium traffic): **$0-10/month**

**Supabase Free**: Up to 500MB database

---

**Chi tiết đầy đủ**: Xem `DEPLOYMENT_GUIDE_STEP_BY_STEP.md`
