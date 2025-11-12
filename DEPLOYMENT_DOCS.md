# 📚 Tài Liệu Triển Khai Google Cloud Run

Hệ thống CPLS Backend đã sẵn sàng triển khai lên Google Cloud Run với đầy đủ tài liệu hướng dẫn.

---

## 🎯 Chọn Phương Thức Triển Khai

### 1. ⚡ Triển Khai Nhanh (15 phút) - RECOMMENDED

**File**: [`QUICK_START_DEPLOY.md`](./QUICK_START_DEPLOY.md)

**Phù hợp với**:
- ✅ Người mới bắt đầu với Google Cloud
- ✅ Muốn triển khai nhanh để test
- ✅ Follow hướng dẫn từng bước đơn giản

**Nội dung**:
- 7 bước ngắn gọn
- Commands copy-paste sẵn
- Supabase database (free tier)
- Đủ để có app chạy production

**Thời gian**: 15 phút

---

### 2. 📖 Hướng Dẫn Chi Tiết (30-45 phút)

**File**: [`DEPLOYMENT_GUIDE_STEP_BY_STEP.md`](./DEPLOYMENT_GUIDE_STEP_BY_STEP.md)

**Phù hợp với**:
- ✅ Muốn hiểu sâu từng bước
- ✅ Setup production đầy đủ
- ✅ Cần cấu hình nâng cao (custom domain, scaling, monitoring)

**Nội dung** (15 sections):
1. Chuẩn bị đầy đủ
2. Thiết lập Google Cloud Project
3. Cài đặt gcloud CLI (Linux/Mac/Windows)
4. Thiết lập Database (Supabase + Cloud SQL)
5. Cấu hình môi trường
6. Triển khai lần đầu
7. Kiểm tra deployment
8. **Cấu hình nâng cao**:
   - Custom domain
   - Auto-scaling
   - Secret Manager
   - VPC Connector
   - Cloud Scheduler
9. Monitoring & Logs
10. Troubleshooting toàn diện
11. Update deployment
12. Production checklist
13. Commands reference
14. Cost estimation
15. Next steps

**Thời gian**: 30-45 phút

---

### 3. 🤖 Automated Script (5 phút)

**File**: [`deploy.sh`](./deploy.sh)

**Phù hợp với**:
- ✅ Đã có Google Cloud account configured
- ✅ Muốn tự động hóa deployment
- ✅ Re-deploy nhiều lần

**Usage**:

```bash
# Basic deployment
./deploy.sh

# With specific project
./deploy.sh --project cpls-stock-trading-123456

# Custom region and service name
./deploy.sh --project my-project --region asia-northeast1 --service my-backend

# Help
./deploy.sh --help
```

**Features**:
- ✅ Auto pre-flight checks
- ✅ Verify go.mod format
- ✅ Fix format issues tự động
- ✅ Enable APIs
- ✅ Grant permissions
- ✅ Build & deploy
- ✅ Health check
- ✅ Show service URL
- ✅ Colored output

**Thời gian**: 5 phút setup + 3-5 phút build

---

## 📋 So Sánh Phương Thức

| Feature | Quick Start | Chi Tiết | Script |
|---------|-------------|----------|--------|
| **Thời gian** | 15 phút | 30-45 phút | 5-8 phút |
| **Độ khó** | Dễ | Trung bình | Rất dễ |
| **Giải thích** | Vừa phải | Đầy đủ | Ít |
| **Automation** | Một phần | Không | Đầy đủ |
| **Production-ready** | Có | Có | Có |
| **Troubleshooting** | Cơ bản | Toàn diện | Auto-fix |
| **Best for** | First-time | Production | Re-deploy |

---

## 🚀 Recommended Workflow

### Lần Đầu Triển Khai

1. **Đọc**: `QUICK_START_DEPLOY.md` để hiểu overview
2. **Follow**: `DEPLOYMENT_GUIDE_STEP_BY_STEP.md` sections 1-7
3. **Verify**: App chạy thành công
4. **Setup**: Advanced configs từ section 8 (nếu cần)

### Lần Sau (Re-deploy)

```bash
# Pull latest code
git pull origin claude/analyze-optimize-code-011CV3EkqVvhUeTi6Z8Ap2gn

# Deploy với script
./deploy.sh
```

---

## 📚 Tài Liệu Khác

### Development Docs

| File | Mô tả |
|------|-------|
| `README.md` | API documentation, project overview |
| `ADMIN_GUIDE.md` | Hướng dẫn sử dụng Admin UI |
| `ANALYSIS_COMPREHENSIVE.md` | Phân tích kiến trúc hệ thống |

### Deployment Docs

| File | Mô tả |
|------|-------|
| `CLOUD_RUN_READY.md` | Cloud Run compatibility summary |
| `DEPLOYMENT_FINAL.md` | Deployment overview |
| `BUILD_VERIFICATION.md` | Docker build troubleshooting |

### Summary Docs

| File | Mô tả |
|------|-------|
| `FINAL_SUMMARY.md` | Complete implementation summary |
| `EXECUTIVE_SUMMARY.md` | High-level overview |

---

## ⚡ Quick Commands

### Deploy

```bash
# Automated (recommended)
./deploy.sh

# Manual
gcloud builds submit --config cloudbuild.yaml
```

### Check Status

```bash
# Get service URL
gcloud run services describe cpls-backend \
  --region=asia-southeast1 \
  --format="value(status.url)"

# View logs
gcloud run services logs tail cpls-backend --region=asia-southeast1

# Check health
curl $(gcloud run services describe cpls-backend --region=asia-southeast1 --format="value(status.url)")/health
```

### Update Config

```bash
# Set environment variables
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --update-env-vars="DB_HOST=xxx,DB_PASSWORD=xxx,JWT_SECRET=xxx"

# Scale
gcloud run services update cpls-backend \
  --region=asia-southeast1 \
  --min-instances=1 --max-instances=10
```

---

## 🆘 Getting Help

### Troubleshooting Steps

1. **Check logs**:
   ```bash
   gcloud run services logs read cpls-backend --region=asia-southeast1 --limit=100
   ```

2. **Verify go.mod**:
   ```bash
   head -10 go.mod
   # Must show: "go 1.23" (NOT "go 1.23.0")
   # Must NOT have: "toolchain" directive
   ```

3. **Test locally**:
   ```bash
   docker build -t cpls-test .
   docker run -p 8080:8080 --env-file .env.production cpls-test
   ```

4. **See**: `DEPLOYMENT_GUIDE_STEP_BY_STEP.md` section 10 (Troubleshooting) cho chi tiết

### Common Issues & Solutions

| Issue | Solution | Doc Reference |
|-------|----------|---------------|
| Build failed | Check go.mod format | DEPLOYMENT_GUIDE section 10.1 |
| Service not starting | Verify env vars | DEPLOYMENT_GUIDE section 10.2 |
| Database connection failed | Check DB credentials | DEPLOYMENT_GUIDE section 10.3 |
| Permission denied | Re-grant IAM roles | DEPLOYMENT_GUIDE section 10.5 |

---

## 💰 Cost Estimate

### Free Tier (Google Cloud)
- Requests: 2M/month
- vCPU time: 360,000 seconds/month
- Memory: 180,000 GiB-seconds/month

### Free Tier (Supabase)
- Database: 500MB
- Bandwidth: 2GB/month

### Expected Costs

| Traffic Level | Requests/month | Cost/month |
|---------------|----------------|------------|
| Development | <100K | **$0** |
| Small Production | 500K | **$0-5** |
| Medium Production | 5M | **$10-20** |
| High Production | 20M+ | **$30-100** |

💡 **Tip**: Set budget alerts trong Google Cloud Console

---

## ✅ Deployment Checklist

### Pre-Deployment

- [ ] Code pushed to branch `claude/analyze-optimize-code-011CV3EkqVvhUeTi6Z8Ap2gn`
- [ ] go.mod format correct (`go 1.23`, no toolchain)
- [ ] Dockerfile verified (golang:1.23-alpine)
- [ ] Database created (Supabase/Cloud SQL)
- [ ] Environment variables prepared

### Deployment

- [ ] Google Cloud Project created
- [ ] APIs enabled (Cloud Run, Cloud Build, Container Registry)
- [ ] IAM permissions granted
- [ ] Build successful
- [ ] Service deployed

### Post-Deployment

- [ ] Health check passing
- [ ] Database connected
- [ ] Admin UI accessible
- [ ] API responding
- [ ] Environment variables set
- [ ] Monitoring configured
- [ ] Alerts set up (optional)

### Production

- [ ] Custom domain configured (optional)
- [ ] SSL/HTTPS working
- [ ] Backup strategy in place
- [ ] Scaling configured
- [ ] Cost monitoring enabled

---

## 🎯 Next Steps After Deployment

1. **Initialize Data**:
   - Open Admin UI: `https://your-service-url/admin`
   - Click "Initialize Stock Data"

2. **Create Strategy**:
   - Go to Strategies → Create Strategy
   - Example: SMA Crossover (20/50)

3. **Run Backtest**:
   - Go to Backtests → Run Backtest
   - Test with historical data

4. **Start Trading Bot**:
   - Go to Trading Bot → Configure
   - Start automated trading

5. **Monitor**:
   - Check logs regularly
   - Set up alerts
   - Monitor costs

---

## 📞 Support Resources

### Documentation
- **This repo**: All deployment docs
- **Google Cloud Docs**: https://cloud.google.com/run/docs
- **Supabase Docs**: https://supabase.com/docs

### Tools
- **Google Cloud Console**: https://console.cloud.google.com
- **Supabase Dashboard**: https://app.supabase.com
- **gcloud CLI**: https://cloud.google.com/sdk/gcloud

### Community
- Google Cloud Run: https://cloud.google.com/run/docs/support
- Supabase Discord: https://discord.supabase.com

---

## 🎉 Ready to Deploy!

**Khuyến nghị cho người mới**:
1. Đọc `QUICK_START_DEPLOY.md` (5 phút)
2. Follow `DEPLOYMENT_GUIDE_STEP_BY_STEP.md` (30 phút)
3. Lần sau dùng `./deploy.sh` (5 phút)

**Khuyến nghị cho người có kinh nghiệm**:
```bash
./deploy.sh --project your-project-id
```

---

**Happy Deploying! 🚀**

*Last updated: 2025-11-12*
*Branch: claude/analyze-optimize-code-011CV3EkqVvhUeTi6Z8Ap2gn*
