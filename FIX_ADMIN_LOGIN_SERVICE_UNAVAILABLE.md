# Fix: Admin Login "Service Unavailable" Error

## Problem Description (Vietnamese)

> chưa đăng nhập tài khoản admin được báo lỗi 'Service Unavailable', các tài khoản admin sẽ được lưu ở bảng 'admin_users' trên superbase. hãy sửa code cho đúng

**English Translation:**
When not logged in, admin account gets 'Service Unavailable' error. Admin accounts should be stored in the 'admin_users' table on Supabase. Please fix the code correctly.

## Root Cause

The issue occurred when:
1. Supabase environment variables (SUPABASE_URL, SUPABASE_SERVICE_KEY) **were** configured in Cloud Run
2. The Supabase auth controller was created successfully
3. BUT the connection test to the `admin_users` table **failed** (table doesn't exist or no permission)
4. The code logged a warning but continued to use Supabase auth anyway
5. When users tried to login, all queries to the missing `admin_users` table failed
6. Result: **"Service Unavailable" (503) error**

### Code Flow Before Fix

```go
// routes/routes.go - OLD CODE (BUGGY)
if supabaseURL != "" && (supabaseAnonKey != "" || supabaseServiceKey != "") {
    if sac, err := admin.NewSupabaseAuthController(); err == nil {
        controllers.supabaseAuthController = sac
        controllers.useSupabaseAuth = true  // ✅ Set to true
        
        // Test connection
        if err := sac.TestConnection(); err != nil {
            log.Printf("Warning: Supabase connection test failed: %v", err)
            // ❌ BUG: Still using Supabase auth even though connection failed!
        }
    }
}
```

**The Bug**: The code set `useSupabaseAuth = true` BEFORE testing the connection. When the connection test failed (admin_users table missing), it just logged a warning but kept using Supabase auth, causing all login attempts to fail.

## The Fix

### Code Changes

Modified `routes/routes.go` in the `initializeAuthControllers()` function:

```go
// routes/routes.go - NEW CODE (FIXED)
if supabaseURL != "" && (supabaseAnonKey != "" || supabaseServiceKey != "") {
    if sac, err := admin.NewSupabaseAuthController(); err == nil {
        // Test connection FIRST before enabling Supabase auth
        if err := sac.TestConnection(); err != nil {
            log.Printf("ERROR: Supabase connection test failed: %v", err)
            log.Printf("ERROR: Cannot access admin_users table on Supabase")
            log.Printf("ERROR: Please ensure the admin_users table exists by running migrations/001_admin_users.sql")
            log.Printf("Falling back to GORM-based authentication")
            // ✅ Don't use Supabase auth if connection test fails
        } else {
            // ✅ Connection test successful - NOW we can use Supabase auth
            controllers.supabaseAuthController = sac
            controllers.useSupabaseAuth = true
            log.Printf("✓ Using Supabase REST API for admin authentication")
            log.Printf("✓ Supabase connection test successful - admin_users table is accessible")
        }
    }
}
```

### Improved Error Messages

Also improved the error message shown to users when authentication is unavailable:

```go
// If no auth controller available
if supabaseConfigured {
    // Supabase is configured but connection failed - likely missing admin_users table
    errorMessage = "Database Error: Cannot access admin_users table on Supabase. " +
        "Please run the migration script (migrations/001_admin_users.sql) in your Supabase SQL Editor. " +
        "See DEPLOYMENT_FIX_ADMIN_LOGIN.md for instructions."
} else {
    // Neither Supabase nor GORM database is available
    errorMessage = "Database not connected. Please wait for the system to initialize or contact your administrator."
}
```

## Solution for Users

If you see the "Service Unavailable" error when trying to login to admin panel, you need to create the `admin_users` table in Supabase.

### Step 1: Run Migration in Supabase

1. Go to your Supabase project: https://app.supabase.com
2. Click on **SQL Editor** in the left sidebar
3. Click **+ New Query**
4. Copy and paste the contents of `migrations/001_admin_users.sql`
5. Click **Run** to execute the migration

### Step 2: Create an Admin User

After creating the table, you need to create at least one admin user:

#### Option A: Use the seed script (Recommended)

1. Open SQL Editor in Supabase
2. Copy and paste the contents of `migrations/002_seed_admin_user.sql`
3. Click **Run** (the script will create a default admin user with username `datvt8x` and password `@abcd4321`)
4. **Important**: Change the default password after first login for security

#### Option B: Manually create admin user

1. Generate a password hash locally:
```bash
cd scripts
go run generate_password_hash.go "YourSecurePassword"
```

2. Insert the admin user in Supabase SQL Editor:
```sql
INSERT INTO admin_users (username, password_hash, email, full_name, role, is_active)
VALUES (
    'admin',
    '$2a$10$YOUR_HASH_HERE',  -- Paste the hash from step 1
    'admin@example.com',
    'Administrator',
    'superadmin',
    true
);
```

### Step 3: Verify

1. Restart your application (or wait for Cloud Run to restart)
2. Try logging in at `/admin/login`
3. You should now be able to login successfully

## Technical Details

### Authentication Priority

The system uses this priority for admin authentication:

1. **Supabase REST API** (if configured AND connection test succeeds)
   - Requires: `SUPABASE_URL` + `SUPABASE_SERVICE_KEY` or `SUPABASE_ANON_KEY`
   - Requires: `admin_users` table exists and is accessible
   - Advantage: Works even if PostgreSQL GORM connection fails
   
2. **GORM Database** (if Supabase not available/failed AND database is connected)
   - Requires: `DB_HOST`, `DB_PASSWORD`, etc.
   - Requires: Database connection successful
   
3. **Error Page** (if neither is available)
   - Shows helpful error message
   - Tells user to run migrations if Supabase is configured

### Connection Test

The `TestConnection()` function in `services/supabase_db.go` performs a simple query:

```go
func (c *SupabaseDBClient) TestConnection() error {
    queryURL := fmt.Sprintf("%s/rest/v1/admin_users?limit=0", c.URL)
    // ... makes HTTP request ...
    // Returns error if status != 200
}
```

This query will fail if:
- The `admin_users` table doesn't exist
- The API key doesn't have permission to access the table
- The Supabase URL is wrong
- Network connectivity issues

## Files Modified

1. **routes/routes.go**
   - `initializeAuthControllers()`: Only set `useSupabaseAuth = true` if connection test succeeds
   - `SetupAdminRoutes()`: Improved error messages when auth is unavailable

## Testing

### Test Scenario 1: Supabase configured, admin_users table missing

**Before Fix:**
- Login attempt → 503 Service Unavailable (query to missing table fails)

**After Fix:**
- Logs: "ERROR: Cannot access admin_users table on Supabase"
- Falls back to GORM auth (if available)
- Login page shows: "Please run migration script..."

### Test Scenario 2: Supabase configured, admin_users table exists

**Before Fix:**
- Connection test warning logged but ignored
- Login works if table exists

**After Fix:**
- Logs: "✓ Supabase connection test successful"
- Login works correctly

### Test Scenario 3: Supabase not configured

**Before Fix & After Fix:**
- Uses GORM auth (if database available)
- No change in behavior

## Environment Variables

Make sure these are set correctly in Cloud Run:

```bash
# Required for Supabase Auth
SUPABASE_URL=https://xxxxxxxxxxxxx.supabase.co
SUPABASE_SERVICE_KEY=eyJhbGci...  # Service role key (has admin permissions)

# Optional - can also use anon key
SUPABASE_ANON_KEY=eyJhbGci...

# Fallback to GORM if Supabase fails
DB_HOST=db.xxxxxxxxxxxxx.supabase.co
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your-password
DB_NAME=postgres
```

## Migration Files

The required migration files are located in the `migrations/` directory:

- **001_admin_users.sql** - Creates the `admin_users` and `admin_sessions` tables
- **002_seed_admin_user.sql** - Seeds a default admin user (optional)

## Summary

**What was wrong:**
- Supabase auth was used even when the `admin_users` table didn't exist
- Connection test failures were ignored
- Users got cryptic "Service Unavailable" errors

**What was fixed:**
- Connection test is now mandatory before using Supabase auth
- Clear error messages guide users to run migrations
- Proper fallback to GORM auth if Supabase fails
- Better logging for debugging

**Result:**
- Users see clear instructions when `admin_users` table is missing
- System automatically falls back to GORM if Supabase is not working
- More reliable admin authentication
