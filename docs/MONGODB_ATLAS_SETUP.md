# MongoDB Atlas & Google Cloud Run Configuration Guide

## Overview

This guide explains how to set up MongoDB Atlas for persistent storage of stock data in the CPLS Backend. MongoDB Atlas provides cloud-based persistence that survives Google Cloud Run deployments, ensuring your Stock List, Price Data, and Technical Indicators are never lost.

## Data Storage Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                        DATA STORAGE FLOW                              │
├─────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  1. NEW DATA FETCHED (from VNDirect API)                             │
│     ↓                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐          │
│  │ Local File  │  │   DuckDB    │  │   MongoDB Atlas     │          │
│  │ (Fastest)   │  │ (Local DB)  │  │ (Cloud Persistent)  │          │
│  └─────────────┘  └─────────────┘  └─────────────────────┘          │
│                                                                       │
│  2. ON REDEPLOY (local data lost)                                    │
│     ↓                                                                 │
│  ┌─────────────────────┐     ┌─────────────────────┐                 │
│  │   MongoDB Atlas     │ --> │ Restore to Local    │                 │
│  │ (Cloud Persistent)  │     │ (Auto on startup)   │                 │
│  └─────────────────────┘     └─────────────────────┘                 │
│                                                                       │
└─────────────────────────────────────────────────────────────────────┘
```

## Part 1: MongoDB Atlas Setup

### Step 1: Create MongoDB Atlas Account

1. Go to [MongoDB Atlas](https://www.mongodb.com/atlas)
2. Click "Try Free" and create an account
3. Choose your cloud provider (any is fine, recommend **AWS** or **Google Cloud**)

### Step 2: Create a Cluster

1. Click "Build a Database"
2. Select **M0 FREE** tier (sufficient for this use case)
3. Choose a cloud provider and region:
   - Recommend: **Google Cloud** → **asia-southeast1 (Singapore)** for Vietnam users
4. Name your cluster (e.g., `cpls-cluster`)
5. Click "Create Cluster"

### Step 3: Create Database User

1. Go to **Security** → **Database Access**
2. Click "Add New Database User"
3. Choose **Password** authentication
4. Set username: `cpls_admin` (or your choice)
5. Set a strong password (save this!)
6. Under "Database User Privileges", select **Read and write to any database**
7. Click "Add User"

### Step 4: Configure Network Access

1. Go to **Security** → **Network Access**
2. Click "Add IP Address"
3. For Cloud Run (dynamic IPs), click **"Allow Access from Anywhere"** (0.0.0.0/0)
   - Note: This is required for Cloud Run as it uses dynamic IPs
   - MongoDB Atlas has built-in authentication, so this is safe
4. Click "Confirm"

### Step 5: Get Connection String

1. Go to **Deployment** → **Database**
2. Click "Connect" on your cluster
3. Choose "Connect your application"
4. Select **Driver**: Go, **Version**: 1.8 or later
5. Copy the connection string, it looks like:
   ```
   mongodb+srv://<username>:<password>@cpls-cluster.xxxxx.mongodb.net/?retryWrites=true&w=majority
   ```
6. Replace `<username>` and `<password>` with your actual credentials

### Step 6: Create Database (Automatic)

The database and collections will be created automatically when the application first connects. The following collections will be created:
- `cpls_stock.stock_list` - Stock list from VNDirect
- `cpls_stock.price_data` - Price data for each stock
- `cpls_stock.indicators` - Technical indicators summary

## Part 2: Google Cloud Run Configuration

### Step 1: Set Environment Variable

In Google Cloud Console:

1. Go to **Cloud Run** → Select your service
2. Click **"Edit & Deploy New Revision"**
3. Go to **"Variables & Secrets"** tab
4. Click **"Add Variable"**
5. Add the following:
   - **Name**: `MONGODB_URI`
   - **Value**: Your MongoDB connection string from Step 5 above
   ```
   mongodb+srv://cpls_admin:YOUR_PASSWORD@cpls-cluster.xxxxx.mongodb.net/?retryWrites=true&w=majority
   ```
6. Click **"Deploy"**

### Step 2: Using Secret Manager (Recommended for Production)

For better security, use Google Cloud Secret Manager:

1. Go to **Security** → **Secret Manager**
2. Click **"Create Secret"**
3. Name: `MONGODB_URI`
4. Value: Your MongoDB connection string
5. Click "Create"

Then in Cloud Run:
1. Edit your service
2. Go to **"Variables & Secrets"**
3. Click **"Reference a Secret"**
4. Select `MONGODB_URI` secret
5. Choose **"Exposed as environment variable"**
6. Variable name: `MONGODB_URI`
7. Deploy

### Step 3: Verify Connection

After deploying, check the logs:

```bash
gcloud run logs read --service=cpls-backend --limit=50
```

You should see:
```
MongoDB Atlas initialized successfully
```

Or in the Cloud Console:
1. Go to Cloud Run → Your service → Logs
2. Look for "MongoDB Atlas initialized successfully"

## Part 3: How It Works

### On Startup (After Redeploy)

1. Application starts
2. MongoDB client initializes and connects to Atlas
3. Checks if local price data exists
4. If missing (after redeploy), restores from MongoDB Atlas:
   - Stock list
   - All price data files
   - Technical indicators
5. Data is cached locally for fast access

### When New Data is Fetched

1. Data is fetched from VNDirect API
2. Saved to local file (fastest access)
3. Saved to DuckDB (local database)
4. **Async saved to MongoDB Atlas** (cloud persistence)

### Data Hierarchy (Fallback Order)

1. **Local File** (fastest, lost on redeploy)
2. **DuckDB** (fast, lost on redeploy)
3. **MongoDB Atlas** (persistent, survives redeploy)
4. **Supabase** (legacy fallback)

## Part 4: Manual Sync Operations

### Admin API Endpoints

The following API endpoints are available for manual sync operations:

```bash
# Sync all local data to MongoDB Atlas
POST /admin/api/stocks/sync-to-mongodb

# Restore all data from MongoDB Atlas
POST /admin/api/stocks/restore-from-mongodb

# Get MongoDB status
GET /admin/api/stocks/mongodb-status
```

### Using the Admin Dashboard

1. Login to Admin Dashboard: `/admin`
2. Go to **Stocks** page
3. Use the sync buttons to manually trigger sync operations

## Part 5: Troubleshooting

### Connection Errors

**Error**: `failed to connect to MongoDB Atlas`
- Check if `MONGODB_URI` is set correctly
- Verify username/password in connection string
- Check network access allows 0.0.0.0/0

**Error**: `authentication failed`
- Verify database user credentials
- Check if user has read/write permissions

### Data Not Restoring

**Issue**: Data not restored after redeploy
- Check logs for "Restoring price data from MongoDB Atlas..."
- Verify MongoDB has data: check Atlas dashboard → Collections

### Slow Startup

**Issue**: Slow startup time
- This is normal for first-time data restore (hundreds of stocks)
- Subsequent starts will be faster as data is cached locally

## Part 6: Cost Considerations

### MongoDB Atlas M0 (Free Tier)

- **Storage**: 512 MB
- **Connections**: 500 concurrent
- **Price**: FREE

This is sufficient for:
- ~1500 stocks with 1 year of price history
- Technical indicators for all stocks

### Upgrading

If you need more storage:
- M2 (Shared): $9/month, 2 GB storage
- M5 (Shared): $25/month, 5 GB storage
- M10 (Dedicated): $57/month, 10 GB storage

## Part 7: Best Practices

1. **Use Secret Manager** for production environments
2. **Monitor Atlas metrics** for connection usage
3. **Set up alerts** for storage approaching limits
4. **Regular backups** - Atlas provides automated backups on paid tiers
5. **Use connection pooling** - Already configured in the code (max 10 connections)

## Environment Variables Summary

| Variable | Required | Description |
|----------|----------|-------------|
| `MONGODB_URI` | Yes | MongoDB Atlas connection string |
| `SUPABASE_URL` | Yes | Supabase project URL |
| `SUPABASE_KEY` | Yes | Supabase anon/service key |

## Testing the Setup

1. Deploy to Cloud Run with `MONGODB_URI` set
2. Login to admin dashboard
3. Sync stock list from VNDirect
4. Sync price data for stocks
5. Calculate technical indicators
6. Redeploy the service
7. Verify data is restored from MongoDB Atlas

The data should be available immediately after redeploy without needing to re-fetch from APIs.
