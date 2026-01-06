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
    -- Bcrypt hash for password '@abcd4321' - CHANGE IN PRODUCTION!
    default_password_hash VARCHAR(255) := '$2a$10$soFYLqqzyNrRdQW39U9u2u/ZRG0c8f4xYrtqeOpu/BCtoSjK5Ef6q';
BEGIN
    -- Count existing admin users
    SELECT COUNT(*) INTO admin_count FROM admin_users;
    
    -- Only seed if no admin users exist
    IF admin_count = 0 THEN
        RAISE NOTICE '=============================================================';
        RAISE NOTICE 'Creating default admin user with default credentials.';
        RAISE NOTICE 'IMPORTANT: Change the password after first login!';
        RAISE NOTICE 'To generate a new password hash, run:';
        RAISE NOTICE '  go run scripts/generate_password_hash.go "YourPassword"';
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
