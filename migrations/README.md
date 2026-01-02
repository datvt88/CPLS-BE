# Database Migrations for Supabase

This directory contains SQL migration scripts for setting up the database schema in Supabase.

## Migration Files

1. **001_admin_users.sql** - Creates admin_users and admin_sessions tables
2. **002_seed_admin_user.sql** - Seeds the default admin user
3. **003_stock_prices_indicators.sql** - Creates stock data tables

## How to Apply Migrations to Supabase

### Option 1: Using Supabase SQL Editor (Web Console)

1. Go to your Supabase project: https://app.supabase.com
2. Navigate to **SQL Editor** in the left sidebar
3. Click **+ New Query**
4. Copy the content of each migration file and paste it into the SQL editor
5. Click **Run** to execute
6. Repeat for each migration file in order (001, 002, 003)

### Option 2: Using Supabase CLI

```bash
# Install Supabase CLI
npm install -g supabase

# Login to Supabase
supabase login

# Link to your project
supabase link --project-ref YOUR_PROJECT_REF

# Apply migrations
supabase db push
```

### Option 3: Manual SQL Execution via psql

```bash
# Connect to your Supabase PostgreSQL database
psql "postgresql://postgres:YOUR_PASSWORD@db.YOUR_PROJECT_REF.supabase.co:5432/postgres"

# Run each migration
\i migrations/001_admin_users.sql
\i migrations/002_seed_admin_user.sql
\i migrations/003_stock_prices_indicators.sql
```

## Setting Up Default Admin User

The default admin user credentials should be set via environment variables:

- `ADMIN_DEFAULT_USERNAME` (default: `datvt8x`)
- `ADMIN_DEFAULT_PASSWORD` (default: `@abcd4321`)

### Generate Password Hash

To generate a bcrypt password hash for the default admin user:

```bash
# Using Go script
cd scripts
go run generate_password_hash.go "YourSecurePassword"
```

This will output a bcrypt hash that you can use in the migration or insert directly into Supabase.

### Insert Admin User with Custom Credentials

If you want to manually insert an admin user with custom credentials:

```sql
-- Generate a bcrypt hash for your password first (cost=10)
-- Then insert:
INSERT INTO admin_users (username, password_hash, email, full_name, role, is_active)
VALUES (
    'your_username',
    '$2a$10$YOUR_BCRYPT_HASH_HERE',
    'your@email.com',
    'Your Full Name',
    'superadmin',
    true
);
```

## Verifying Migrations

After applying migrations, verify the tables exist:

```sql
-- Check if admin_users table exists
SELECT * FROM admin_users;

-- Check if admin_sessions table exists
SELECT * FROM admin_sessions;

-- Check if stock tables exist
SELECT * FROM stock_prices LIMIT 1;
SELECT * FROM stock_indicators LIMIT 1;
```

## Environment Variables Required on Cloud Run

Make sure these environment variables are set on Google Cloud Run:

```bash
# Supabase Connection
SUPABASE_URL=https://your-project-ref.supabase.co
SUPABASE_ANON_KEY=your-anon-key
SUPABASE_SERVICE_KEY=your-service-role-key

# Database Connection (for GORM fallback)
DB_HOST=db.your-project-ref.supabase.co
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your-db-password
DB_NAME=postgres

# Admin Credentials
ADMIN_DEFAULT_USERNAME=datvt8x
ADMIN_DEFAULT_PASSWORD=YourSecurePassword

# Application Config
JWT_SECRET=your-jwt-secret-key
ENVIRONMENT=production
PORT=8080
```

## Troubleshooting

### Error: relation "admin_users" does not exist

Run migration 001_admin_users.sql first.

### Error: Invalid password or user not found

1. Check if admin user exists: `SELECT * FROM admin_users;`
2. If no users exist, run 002_seed_admin_user.sql or manually insert an admin user
3. Verify password hash is correct by using the generate_password_hash.go script

### Error: Connection to Supabase failed

1. Verify SUPABASE_URL is correct
2. Verify SUPABASE_SERVICE_KEY or SUPABASE_ANON_KEY is set
3. Check Supabase project is active and not paused
4. Test connection using the Supabase dashboard

## Security Notes

⚠️ **IMPORTANT:**
- Never commit passwords or API keys to git
- Use strong passwords for admin accounts
- Rotate JWT secrets periodically
- Use Google Secret Manager for production credentials
- Enable Row Level Security (RLS) in Supabase for additional protection
