# Tóm tắt Sửa lỗi Admin Login (Vietnamese Summary)

## Vấn đề ban đầu

**Mô tả lỗi (bằng tiếng Việt):**
> chưa đăng nhập tài khoản admin được báo lỗi 'Service Unavailable', các tài khoản admin sẽ được lưu ở bảng 'admin_users' trên superbase. hãy sửa code cho đúng

**Dịch:**
Khi chưa đăng nhập, tài khoản admin bị báo lỗi "Service Unavailable". Các tài khoản admin được lưu ở bảng `admin_users` trên Supabase. Hãy sửa code cho đúng.

## Nguyên nhân

Hệ thống có vấn đề khi:
1. ✅ Các biến môi trường Supabase (SUPABASE_URL, SUPABASE_SERVICE_KEY) đã được cấu hình trên Cloud Run
2. ✅ Supabase auth controller được tạo thành công
3. ❌ NHƯNG test kết nối đến bảng `admin_users` bị **thất bại** (bảng không tồn tại hoặc không có quyền truy cập)
4. ❌ Code chỉ ghi log cảnh báo nhưng vẫn tiếp tục sử dụng Supabase auth
5. ❌ Khi user cố gắng đăng nhập, tất cả các truy vấn đến bảng `admin_users` bị lỗi
6. ❌ **Kết quả: Lỗi "Service Unavailable" (503)**

## Giải pháp đã thực hiện

### 1. Sửa logic trong `routes/routes.go`

**TRƯỚC KHI SỬA (BUG):**
```go
// Tạo Supabase auth controller
controllers.supabaseAuthController = sac
controllers.useSupabaseAuth = true  // ✅ Đặt thành true

// Test kết nối
if err := sac.TestConnection(); err != nil {
    log.Printf("Warning: Supabase connection test failed: %v", err)
    // ❌ LỖI: Vẫn sử dụng Supabase auth mặc dù kết nối thất bại!
}
```

**SAU KHI SỬA (ĐÚNG):**
```go
// Test kết nối TRƯỚC
if err := sac.TestConnection(); err != nil {
    log.Printf("ERROR: Cannot access admin_users table on Supabase")
    log.Printf("ERROR: Please run migrations/001_admin_users.sql")
    log.Printf("Falling back to GORM authentication")
    // ✅ KHÔNG sử dụng Supabase auth nếu test thất bại
} else {
    // ✅ Chỉ khi test thành công mới sử dụng Supabase auth
    controllers.supabaseAuthController = sac
    controllers.useSupabaseAuth = true
}
```

### 2. Thêm thông báo lỗi rõ ràng

Khi không có auth controller nào khả dụng:
- **Nếu Supabase đã cấu hình**: Hiển thị hướng dẫn chạy migration
- **Nếu Supabase chưa cấu hình**: Hiển thị thông báo database chưa kết nối

### 3. Thêm tài liệu hướng dẫn

Tạo file `FIX_ADMIN_LOGIN_SERVICE_UNAVAILABLE.md` với:
- Giải thích chi tiết vấn đề và nguyên nhân
- Hướng dẫn từng bước để khắc phục
- Chi tiết kỹ thuật về cách authentication hoạt động

## Cách khắc phục cho người dùng

Nếu bạn thấy lỗi "Service Unavailable" khi đăng nhập admin, làm theo các bước sau:

### Bước 1: Chạy Migration trong Supabase

1. Truy cập Supabase project của bạn: https://app.supabase.com
2. Click vào **SQL Editor** ở sidebar bên trái
3. Click **+ New Query**
4. Copy và paste nội dung của file `migrations/001_admin_users.sql`
5. Click **Run** để thực thi

### Bước 2: Tạo tài khoản Admin

Sau khi tạo bảng, bạn cần tạo ít nhất một tài khoản admin:

#### Cách A: Sử dụng seed script (Khuyến nghị)

1. Mở SQL Editor trong Supabase
2. Copy và paste nội dung của `migrations/002_seed_admin_user.sql`
3. Click **Run**
4. Mặc định sẽ tạo user: `datvt8x` / password: `@abcd4321`
5. **Quan trọng**: Đổi mật khẩu sau khi đăng nhập lần đầu

#### Cách B: Tạo admin user thủ công

1. Tạo password hash cục bộ:
```bash
cd scripts
go run generate_password_hash.go "MatKhauCuaBan"
```

2. Insert admin user trong Supabase SQL Editor:
```sql
INSERT INTO admin_users (username, password_hash, email, full_name, role, is_active)
VALUES (
    'admin',
    '$2a$10$YOUR_HASH_HERE',  -- Paste hash từ bước 1
    'admin@example.com',
    'Quản trị viên',
    'superadmin',
    true
);
```

### Bước 3: Xác nhận

1. Khởi động lại ứng dụng (hoặc đợi Cloud Run tự động khởi động lại)
2. Thử đăng nhập tại `/admin/login`
3. Bạn sẽ có thể đăng nhập thành công

## Chi tiết kỹ thuật

### Thứ tự ưu tiên Authentication

Hệ thống sử dụng thứ tự ưu tiên sau cho admin authentication:

1. **Supabase REST API** (nếu được cấu hình VÀ test kết nối thành công)
   - Yêu cầu: `SUPABASE_URL` + `SUPABASE_SERVICE_KEY` hoặc `SUPABASE_ANON_KEY`
   - Yêu cầu: Bảng `admin_users` tồn tại và có thể truy cập
   - Ưu điểm: Hoạt động ngay cả khi kết nối PostgreSQL GORM bị lỗi

2. **GORM Database** (nếu Supabase không khả dụng/thất bại VÀ database đã kết nối)
   - Yêu cầu: `DB_HOST`, `DB_PASSWORD`, v.v.
   - Yêu cầu: Kết nối database thành công

3. **Trang lỗi** (nếu cả hai không khả dụng)
   - Hiển thị thông báo lỗi hữu ích
   - Hướng dẫn user chạy migration nếu Supabase đã được cấu hình

## Files đã sửa đổi

1. **routes/routes.go**
   - `initializeAuthControllers()`: Chỉ đặt `useSupabaseAuth = true` nếu test kết nối thành công
   - `SetupAdminRoutes()`: Cải thiện thông báo lỗi khi auth không khả dụng

2. **FIX_ADMIN_LOGIN_SERVICE_UNAVAILABLE.md** (Mới)
   - Tài liệu toàn diện về vấn đề và giải pháp

3. **SUMMARY_VIETNAMESE.md** (File này)
   - Tóm tắt bằng tiếng Việt cho người dùng Việt Nam

## Kiểm tra bảo mật

✅ **CodeQL Security Scan**: 0 lỗ hổng bảo mật được tìm thấy

## Kết quả

Sau khi sửa, hệ thống sẽ:
- ✅ Hiển thị thông báo lỗi rõ ràng khi bảng admin_users bị thiếu
- ✅ Tự động chuyển về GORM auth nếu Supabase thất bại
- ✅ Hướng dẫn user chạy migration với chỉ dẫn cụ thể
- ✅ Hoạt động ổn định khi được cấu hình đúng

## Biến môi trường cần thiết

Đảm bảo các biến sau được đặt đúng trên Cloud Run:

```bash
# Bắt buộc cho Supabase Auth
SUPABASE_URL=https://xxxxxxxxxxxxx.supabase.co
SUPABASE_SERVICE_KEY=eyJhbGci...  # Service role key (có quyền admin)

# Tùy chọn - cũng có thể dùng anon key
SUPABASE_ANON_KEY=eyJhbGci...

# Dự phòng cho GORM nếu Supabase thất bại
DB_HOST=db.xxxxxxxxxxxxx.supabase.co
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your-password
DB_NAME=postgres
```

## Triển khai

Các thay đổi này đã sẵn sàng để triển khai lên production. Sau khi deploy:

1. Đảm bảo đã chạy migration `001_admin_users.sql` trong Supabase
2. Tạo ít nhất một admin user
3. Khởi động lại service (hoặc deploy mã mới)
4. Kiểm tra đăng nhập tại `/admin/login`

## Liên hệ

Nếu có vấn đề gì, tham khảo file `FIX_ADMIN_LOGIN_SERVICE_UNAVAILABLE.md` để biết thêm chi tiết.
