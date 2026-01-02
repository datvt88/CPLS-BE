-- Seed default admin user
-- This script should be run after the admin_users table is created
-- It creates a default admin user with credentials from environment variables

-- Check if any admin users exist
DO $$
DECLARE
    admin_count INTEGER;
    default_username VARCHAR(100) := COALESCE(current_setting('app.admin_default_username', true), 'datvt8x');
    default_password_hash VARCHAR(255) := COALESCE(current_setting('app.admin_default_password_hash', true), '$2a$10$YourHashHere');
BEGIN
    -- Count existing admin users
    SELECT COUNT(*) INTO admin_count FROM admin_users;
    
    -- Only seed if no admin users exist
    IF admin_count = 0 THEN
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
    ELSE
        RAISE NOTICE 'Admin users already exist (count: %), skipping seed', admin_count;
    END IF;
END $$;
