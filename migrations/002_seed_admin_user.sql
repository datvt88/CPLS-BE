-- Seed default admin user
-- This script should be run after the admin_users table is created
-- It creates a default admin user with credentials from environment variables

-- IMPORTANT: Before running this script, generate a bcrypt hash for your password:
-- Run: go run scripts/generate_password_hash.go "YourSecurePassword"
-- Then replace the hash below with the generated hash

DO $$
DECLARE
    admin_count INTEGER;
    default_username VARCHAR(100) := COALESCE(current_setting('app.admin_default_username', true), 'datvt8x');
    -- REPLACE THIS HASH WITH YOUR ACTUAL BCRYPT HASH BEFORE RUNNING
    -- This is a bcrypt hash for password '@abcd4321' (default) - CHANGE IN PRODUCTION!
    default_password_hash VARCHAR(255) := '$2a$10$8QXQ7J8ZY0X8X8X8X8X8XeJX8X8X8X8X8X8X8X8X8X8X8X8X8X8X8';
BEGIN
    -- Count existing admin users
    SELECT COUNT(*) INTO admin_count FROM admin_users;
    
    -- Only seed if no admin users exist
    IF admin_count = 0 THEN
        RAISE NOTICE '=============================================================';
        RAISE NOTICE 'WARNING: Using placeholder password hash!';
        RAISE NOTICE 'Please generate a proper bcrypt hash using:';
        RAISE NOTICE '  go run scripts/generate_password_hash.go "YourPassword"';
        RAISE NOTICE 'Then update this script with the generated hash';
        RAISE NOTICE '=============================================================';
        
        -- Insert default admin user
        INSERT INTO admin_users (username, password_hash, email, full_name, role, is_active)
        VALUES (
            default_username,
            default_password_hash,
            'admin@cpls.com',
            'Administrator',
            'superadmin',
            true
        );
        
        RAISE NOTICE 'Default admin user created: %', default_username;
        RAISE NOTICE 'REMEMBER TO CHANGE THE PASSWORD AFTER FIRST LOGIN!';
    ELSE
        RAISE NOTICE 'Admin users already exist (count: %), skipping seed', admin_count;
    END IF;
END $$;
