# Hướng dẫn Sửa lỗi Admin Login với Supabase

## Vấn đề đã xác định

Hệ thống không thể đăng nhập vào trang admin do lỗi kết nối database. Mặc dù các keys Supabase (`SUPABASE_SERVICE_KEY`, `SUPABASE_URL`, `SUPABASE_ANON_KEY`) đã được lưu trữ trên Google Cloud Run, nhưng hệ thống vẫn đang sử dụng GORM database authentication thay vì Supabase REST API.

## Giải pháp đã implement

Hệ thống đã được cập nhật để:
1. **Tự động phát hiện** Supabase environment variables
2. **Ưu tiên sử dụng** Supabase REST API để authenticate (không cần GORM)
3. **Fallback** về GORM authentication nếu Supabase không có sẵn

## Các bước Deploy

### Bước 1: Apply Database Migrations vào Supabase

#### Cách 1: Sử dụng Supabase SQL Editor (Khuyến nghị - Dễ nhất)

1. Truy cập Supabase project của bạn: https://app.supabase.com
2. Chọn project → Click vào **SQL Editor** ở sidebar bên trái
3. Click **+ New Query**
4. Copy và paste nội dung từ file `migrations/001_admin_users.sql` vào editor
5. Click **Run** để thực thi
6. Lặp lại với file `migrations/003_stock_prices_indicators.sql` (nếu chưa có)

#### Cách 2: Sử dụng psql (Command line)

```bash
# Kết nối vào Supabase database
# Thay YOUR_PROJECT_REF và YOUR_PASSWORD bằng thông tin của bạn
psql "postgresql://postgres:YOUR_PASSWORD@db.YOUR_PROJECT_REF.supabase.co:5432/postgres"

# Trong psql, chạy migrations
\i migrations/001_admin_users.sql
\i migrations/003_stock_prices_indicators.sql
```

### Bước 2: Tạo Default Admin User

#### Tùy chọn A: Generate password hash và insert thủ công (Khuyến nghị)

1. Generate bcrypt hash cho password của bạn:
```bash
cd scripts
go run generate_password_hash.go "YourSecurePassword"
```

2. Copy hash được tạo ra, sau đó insert vào Supabase SQL Editor:
```sql
INSERT INTO admin_users (username, password_hash, email, full_name, role, is_active)
VALUES (
    'datvt8x',
    '$2a$10$YOUR_HASH_HERE',  -- Paste hash vào đây
    'admin@cpls.com',
    'Administrator',
    'superadmin',
    true
);
```

#### Tùy chọn B: Sử dụng seed script

1. Mở Supabase SQL Editor
2. Copy nội dung từ `migrations/002_seed_admin_user.sql`
3. **CHÚ Ý**: Sửa dòng `default_password_hash` trong script để dùng hash thật của bạn
4. Run script

### Bước 3: Verify Environment Variables trên Cloud Run

Đảm bảo các environment variables sau đã được set trên Google Cloud Run:

```bash
# Supabase Keys (BẮT BUỘC cho Supabase Authentication)
SUPABASE_URL=https://xxxxxxxxxxxxx.supabase.co
SUPABASE_ANON_KEY=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
SUPABASE_SERVICE_KEY=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

# Database Connection (Optional - Fallback cho GORM)
DB_HOST=db.xxxxxxxxxxxxx.supabase.co
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your-password
DB_NAME=postgres

# Application Config
JWT_SECRET=your-jwt-secret
ENVIRONMENT=production
PORT=8080
```

#### Kiểm tra Cloud Run Environment Variables:

1. Truy cập: https://console.cloud.google.com/run
2. Chọn service `cpls-backend`
3. Click **EDIT & DEPLOY NEW REVISION**
4. Tab **VARIABLES & SECRETS**
5. Verify các keys sau tồn tại và có giá trị đúng:
   - ✅ `SUPABASE_URL`
   - ✅ `SUPABASE_ANON_KEY` hoặc `SUPABASE_SERVICE_KEY`

### Bước 4: Deploy Code mới lên Cloud Run

#### Cách 1: Auto Deploy (nếu đã setup Cloud Build trigger)

Code sẽ tự động deploy khi merge PR này vào main branch.

#### Cách 2: Manual Deploy

```bash
# Build và deploy
gcloud builds submit --config cloudbuild.yaml

# Hoặc deploy trực tiếp
gcloud run deploy cpls-backend \
  --source . \
  --region asia-southeast1 \
  --platform managed
```

### Bước 5: Test Admin Login

1. Truy cập: `https://your-cloudrun-url.run.app/admin/login`
2. Đăng nhập với credentials:
   - Username: `datvt8x` (hoặc username bạn đã tạo)
   - Password: Password bạn đã dùng để generate hash

## Verify Hoạt động

### Kiểm tra logs để xác nhận sử dụng Supabase Auth:

```bash
gcloud run services logs read cpls-backend --region=asia-southeast1 --limit=50
```

Bạn sẽ thấy log:
```
✓ Using Supabase REST API for admin authentication
✓ Supabase connection test successful
```

Nếu thấy log:
```
Supabase keys not found, using GORM-based authentication
```
→ Có nghĩa là thiếu Supabase environment variables

### Kiểm tra database:

Truy cập Supabase SQL Editor và chạy:
```sql
-- Kiểm tra admin users đã tồn tại
SELECT id, username, email, role, is_active, created_at 
FROM admin_users;

-- Kiểm tra sessions (sau khi login)
SELECT id, admin_user, token, expires_at, created_at 
FROM admin_sessions 
WHERE expires_at > NOW();
```

## Troubleshooting

### Lỗi: "relation 'admin_users' does not exist"
→ Chạy migration `001_admin_users.sql` trong Supabase SQL Editor

### Lỗi: "Invalid username or password"
→ Verify:
1. Admin user đã được tạo: `SELECT * FROM admin_users;`
2. Password hash đúng (dùng `generate_password_hash.go` để tạo lại)

### Lỗi: "SUPABASE_URL is required"
→ Set environment variables trên Cloud Run (xem Bước 3)

### Login vẫn không hoạt động sau khi deploy
→ Check logs:
```bash
gcloud run services logs read cpls-backend --region=asia-southeast1 --limit=100
```

## Thông tin thêm

- **Password mặc định** (nếu dùng seed script): `@abcd4321`
- **Username mặc định**: `datvt8x`
- **Session timeout**: 7 ngày
- **Bcrypt cost**: 10 (default)

## Bảo mật

⚠️ **QUAN TRỌNG:**
- Đổi password mặc định ngay sau khi login lần đầu
- Sử dụng password mạnh (ít nhất 12 ký tự, chữ hoa, chữ thường, số, ký tự đặc biệt)
- Không commit passwords vào git
- Sử dụng Google Secret Manager cho production credentials
- Rotate JWT_SECRET định kỳ

## Tài liệu tham khảo

- Supabase REST API: https://supabase.com/docs/guides/api
- Google Cloud Run: https://cloud.google.com/run/docs
- Bcrypt: https://en.wikipedia.org/wiki/Bcrypt
