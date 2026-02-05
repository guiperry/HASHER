# Hasher Database Specification (KNIRVBASE Edition)

**Version:** 2.0
**Date:** December 24, 2024
**Status:** Architecture Specification
**Classification:** Technical Design Document
**Database:** KNIRVBASE (Distributed NoSQL)

---

## Executive Summary

This document specifies the complete database schema for the Hasher quantum-resistant password vault system using **KNIRVBASE**, a local-first distributed NoSQL database with CRDT-based conflict resolution. The design leverages KNIRVBASE's strengths in offline-first operation, eventual consistency, and P2P synchronization while meeting Hasher's demanding security and performance requirements.

**Key Design Principles:**
- **Security First:** All credential data encrypted at rest with PQC (Kyber-768)
- **Audit Trail:** Immutable logging of all security events with cryptographic signatures
- **Performance:** Optimized for high-velocity authentication (p99 < 500ms)
- **Scalability:** Support 10K-50K users per instance with distributed architecture
- **Compliance:** SOC 2, ISO 27001, GDPR-ready schema
- **Local-First:** Offline-capable with eventual consistency across nodes
- **Distributed:** P2P synchronization for high availability

---

## Table of Contents

1. [Database Selection](#1-database-selection)
2. [Schema Overview](#2-schema-overview)
3. [Core Tables](#3-core-tables)
4. [Monitoring Tables](#4-monitoring-tables)
5. [Security Tables](#5-security-tables)
6. [Indexing Strategy](#6-indexing-strategy)
7. [Encryption Strategy](#7-encryption-strategy)
8. [Backup and Recovery](#8-backup-and-recovery)
9. [Migration Strategy](#9-migration-strategy)
10. [Performance Optimization](#10-performance-optimization)

---

## 1. Database Selection: KNIRVBASE

### 1.1 Why KNIRVBASE for Hasher?

**KNIRVBASE** is a local-first distributed NoSQL database designed for the KNIRV ecosystem with built-in support for:
- **Offline-first operation** - Works without network connectivity
- **Eventual consistency** - CRDT-based conflict resolution using vector clocks
- **P2P synchronization** - Automatic data replication across nodes
- **Document storage** - Flexible JSON-based data model
- **Zero-dependency deployment** - Embedded Go library, no external database server

**Why KNIRVBASE is Perfect for Hasher:**

1. **Security by Design**
   - Built-in support for encryption at rest
   - Integration with PQC (Kyber-768, Dilithium-3)
   - Immutable audit logging (append-only mode)
   - Local-first = reduced attack surface (no external DB to secure)

2. **High Availability**
   - Distributed architecture with P2P sync
   - No single point of failure
   - Automatic failover through CRDT convergence
   - Works offline, syncs when network available

3. **Performance**
   - In-memory caching with file-backed persistence
   - Optimized for authentication workloads (< 500ms p99)
   - ASIC-accelerated KDF integration (same ecosystem)
   - Low latency local operations

4. **Operational Simplicity**
   - No database server to maintain
   - Automatic synchronization (no manual replication setup)
   - Simple deployment (Go binary + data directory)
   - Compatible with Hasher monolithic architecture

**Configuration:**
```go
// KNIRVBASE Configuration for Hasher
db, err := knirvbase.New(ctx, knirvbase.Options{
    DataDir:            "/data/hasher",
    DistributedEnabled: true,

    // Storage Options
    StorageBackend:     "file",  // or "badger" for production
    CacheSize:          67108864, // 64MB cache

    // Security Options
    EncryptionEnabled:  true,
    EncryptionAlgorithm: "AES-256-GCM",
    PQCEnabled:         true,
    PQCAlgorithm:       "Kyber-768",

    // Performance Options
    IndexingEnabled:    true,
    MaxConcurrentOps:   100,
    SyncInterval:       30 * time.Second,

    // Observability
    MetricsEnabled:     true,
    LogLevel:           "info",
})

// Create network for distributed deployment
networkID, err := db.CreateNetwork(types.NetworkConfig{
    NetworkID:   "hasher-production",
    Name:        "Hasher Vault Network",
    Description: "Distributed credential vault",
})

// Create collections
credentialsColl := db.Collection("credentials")
auditLogColl := db.Collection("audit_log")
sessionsColl := db.Collection("sessions")
deviceStatusColl := db.Collection("device_status")

// Attach collections to network
credentialsColl.AttachToNetwork(networkID)
auditLogColl.AttachToNetwork(networkID)
sessionsColl.AttachToNetwork(networkID)
deviceStatusColl.AttachToNetwork(networkID)
```

**Production Deployment:**
- **Single Node**: File-based storage, local-only operation
- **HA Cluster**: 3-5 nodes with P2P sync, distributed across availability zones
- **Hybrid**: Local primary + remote replicas for disaster recovery

---

### 1.2 KNIRVBASE Architecture for Hasher

```
┌─────────────────────────────────────────────────────────────┐
│                  Hasher Application                    │
│            (Go Backend + ASIC Controller)                   │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│               KNIRVBASE Embedded Database                   │
├─────────────────────────────────────────────────────────────┤
│  Collections:                                               │
│  ┌────────────┬────────────┬────────────┬────────────┐      │
│  │ Credentials│ Audit Log  │  Sessions  │ Device     │      │
│  │            │            │            │  Status    │      │
│  └────────────┴────────────┴────────────┴────────────┘      │
│                                                             │
│  Features:                                                  │
│  • PQC Encryption (Kyber-768)                               │
│  • Secondary Indexes (username, email, status, etc.)        │
│  • Immutable Audit Log                                      │
│  • CRDT Conflict Resolution                                 │
│  • P2P Synchronization                                      │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│              Storage Layer (File or Badger)                 │
│  ~/.local/share/hasher/                                │
│  ├── credentials/                                           │
│  │   ├── alice@example.com.json  (encrypted metadata)       │
│  │   └── blobs/                                             │
│  │       └── alice@example.com    (encrypted hash)          │
│  ├── audit_log/                                             │
│  │   └── event-*.json             (signed, immutable)       │
│  ├── sessions/                                              │
│  └── device_status/                                         │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│           P2P Network (libp2p) [Optional for HA]            │
│  • DHT Peer Discovery                                       │
│  • PubSub for Broadcasts                                    │
│  • Direct Streams for Sync                                  │
│  • NAT Traversal                                            │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. Schema Overview

### 2.1 KNIRVBASE Collections Organization

**Note:** KNIRVBASE uses a **document-based** schema (NoSQL) instead of relational tables. Each collection stores JSON documents with flexible schemas. We achieve relational-like behavior through:
- **Document IDs** for primary keys (usernames, UUIDs)
- **Embedded references** for relationships (e.g., `pqc_key_id` embedded in credential documents)
- **Secondary indexes** for fast lookups (username, email, status)
- **CRDT metadata** automatically managed by KNIRVBASE (vector clocks, timestamps)

```
Hasher KNIRVBASE Database
├── Core Collections
│   ├── credentials              # User password storage (EntryType: CREDENTIAL)
│   ├── pqc_keys                 # Post-quantum cryptographic keys (EntryType: PQC_KEY)
│   ├── sessions                 # Active authentication sessions (EntryType: SESSION)
│   └── metadata                 # System configuration (EntryType: CONFIG)
│
├── Monitoring Collections
│   ├── device_status            # ASIC hardware health (EntryType: DEVICE_STATUS)
│   ├── performance_metrics      # KDF performance data (EntryType: METRIC)
│   └── health_checks            # System health snapshots (EntryType: HEALTH_CHECK)
│
├── Security Collections
│   ├── audit_log                # Security event log (EntryType: AUDIT, immutable)
│   ├── rate_limits              # Anti-brute-force tracking (EntryType: RATE_LIMIT)
│   ├── threat_events            # Security incident tracking (EntryType: THREAT)
│   └── access_control           # Role-based permissions (EntryType: ACCESS_CONTROL)
│
└── Operational Collections
    ├── schema_versions          # Schema version tracking (EntryType: SCHEMA_VERSION)
    ├── backup_log               # Backup history (EntryType: BACKUP_LOG)
    └── job_queue                # Async task management (EntryType: JOB)
```

### 2.2 KNIRVBASE Document Structure

All documents in KNIRVBASE follow this standard structure:

```go
type DistributedDocument struct {
    ID        string                 `json:"id"`         // Document ID (username, UUID, etc.)
    EntryType EntryType              `json:"entryType"`  // CREDENTIAL, AUDIT, SESSION, etc.
    Payload   map[string]interface{} `json:"payload"`    // Application-specific data
    Vector    VectorClock            `json:"vector"`     // CRDT vector clock (auto-managed)
    Timestamp int64                  `json:"timestamp"`  // Creation timestamp (ms, auto-managed)
    PeerID    string                 `json:"peerID"`     // Origin peer ID (auto-managed)
    Stage     string                 `json:"stage"`      // "immutable" for audit logs, "" otherwise
    Deleted   bool                   `json:"deleted"`    // Tombstone marker (soft delete)
}
```

**CRDT Metadata** (automatically managed by KNIRVBASE):
- `Vector`: Vector clock for causality tracking (e.g., `{"peer1": 5, "peer2": 3}`)
- `Timestamp`: Unix timestamp in milliseconds
- `PeerID`: Unique identifier of the peer that created this document
- `Deleted`: Soft delete flag (tombstone)

**Application Data** (in `Payload`):
- All application-specific fields go in the `Payload` map
- Type is flexible (strings, numbers, nested objects, arrays)
- No schema enforcement (NoSQL flexibility)
- Optional: Use `entryType` to define schema conventions

### 2.3 Naming Conventions

**Collections:** `snake_case`, plural nouns (e.g., `credentials`, `audit_log`)
**Document IDs:** Context-specific format:
  - **Credentials:** `username` (e.g., `alice@example.com`)
  - **Sessions:** `UUID` (e.g., `550e8400-e29b-41d4-a716-446655440000`)
  - **Audit Events:** `UUID` (e.g., `event-uuid`)
  - **Device Status:** `device_id` (e.g., `antminer-s3-001`)
**Payload Fields:** `snake_case` (e.g., `created_at`, `hash_algorithm`)
**Entry Types:** `UPPER_SNAKE_CASE` (e.g., `CREDENTIAL`, `AUDIT`, `SESSION`)
**Indexes:** Virtual (implemented via KNIRVBASE index layer)
  - `credentials:username` (unique)
  - `credentials:email` (non-unique)
  - `credentials:status` (non-unique)
  - `sessions:session_id` (unique)
  - `audit_log:timestamp` (sorted)

---

## 3. Core Tables

### 3.1 Credentials Collection

**Purpose:** Store user credentials with ASIC-accelerated KDF hashes

#### Collection Creation & Schema Definition

```knirvql
-- Create credentials collection with schema
CREATE COLLECTION credentials WITH SCHEMA {
    "entryType": "CREDENTIAL",
    "fields": {
        "username": {
            "type": "string",
            "required": true,
            "unique": true,
            "maxLength": 255
        },
        "display_name": {
            "type": "string",
            "maxLength": 255
        },
        "email": {
            "type": "string",
            "format": "email",
            "maxLength": 255
        },
        "hash": {
            "type": "binary",
            "required": true,
            "encrypted": true,
            "encryptionAlgorithm": "PQC-Kyber768"
        },
        "salt": {
            "type": "binary",
            "required": true,
            "minLength": 32,
            "maxLength": 32
        },
        "iterations": {
            "type": "integer",
            "required": true,
            "min": 10000000,
            "max": 500000000,
            "default": 100000000
        },
        "algorithm": {
            "type": "string",
            "required": true,
            "default": "PBKDF2-SHA256-ASIC",
            "enum": ["PBKDF2-SHA256-ASIC", "Argon2id-ASIC"]
        },
        "pqc_algorithm": {
            "type": "string",
            "required": true,
            "default": "Kyber-768",
            "enum": ["Kyber-768", "Kyber-1024"]
        },
        "pqc_key_id": {
            "type": "string",
            "format": "uuid",
            "references": "pqc_keys"
        },
        "metadata": {
            "type": "object",
            "default": {}
        },
        "created_at": {
            "type": "timestamp",
            "required": true,
            "autoGenerate": true
        },
        "updated_at": {
            "type": "timestamp",
            "required": true,
            "autoUpdate": true
        },
        "last_used": {
            "type": "timestamp"
        },
        "expires_at": {
            "type": "timestamp"
        },
        "status": {
            "type": "string",
            "required": true,
            "default": "active",
            "enum": ["active", "locked", "expired", "disabled"]
        },
        "failed_attempts": {
            "type": "integer",
            "required": true,
            "default": 0,
            "min": 0
        },
        "locked_until": {
            "type": "timestamp"
        }
    }
}

-- Create indexes
CREATE INDEX credentials:username ON credentials (username) UNIQUE
CREATE INDEX credentials:email ON credentials (email)
CREATE INDEX credentials:status ON credentials (status) WHERE status = "active"
CREATE INDEX credentials:last_used ON credentials (last_used) ORDER BY DESC
CREATE INDEX credentials:created_at ON credentials (created_at) ORDER BY DESC
CREATE INDEX credentials:metadata ON credentials (metadata) TYPE GIN

-- Create auto-update trigger
CREATE TRIGGER credentials:auto_update_timestamp
    BEFORE UPDATE ON credentials
    SET updated_at = NOW()
```

#### KNIRVQL Operations

**Insert (Store) Credential:**
```knirvql
-- Insert new credential
INSERT INTO credentials
SET username = "alice@example.com",
    display_name = "Alice Johnson",
    email = "alice@example.com",
    hash = @pqc_encrypted_hash,
    salt = @random_salt,
    iterations = 100000000,
    algorithm = "PBKDF2-SHA256-ASIC",
    pqc_algorithm = "Kyber-768",
    pqc_key_id = "550e8400-e29b-41d4-a716-446655440000",
    metadata = {
        "department": "engineering",
        "role": "admin",
        "mfa_enabled": true
    },
    status = "active"
```

**Query Credential (Authentication):**
```knirvql
-- Find credential by username (uses index)
GET CREDENTIAL FROM credentials
WHERE username = "alice@example.com"
LIMIT 1
```

**Query with Complex Filters:**
```knirvql
-- Find all active credentials created in last 30 days
GET CREDENTIAL FROM credentials
WHERE status = "active"
  AND created_at > NOW() - INTERVAL 30 DAYS
ORDER BY created_at DESC
LIMIT 100

-- Find locked accounts
GET CREDENTIAL FROM credentials
WHERE status = "locked"
  AND locked_until > NOW()
ORDER BY locked_until ASC

-- Find credentials by metadata (admin users only)
GET CREDENTIAL FROM credentials
WHERE metadata.role = "admin"
  AND status = "active"
```

**Update Credential:**
```knirvql
-- Update last_used timestamp after successful authentication
UPDATE credentials
SET last_used = NOW(),
    failed_attempts = 0
WHERE username = "alice@example.com"

-- Increment failed attempts
UPDATE credentials
SET failed_attempts = failed_attempts + 1
WHERE username = "alice@example.com"

-- Lock account after too many failures
UPDATE credentials
SET status = "locked",
    locked_until = NOW() + INTERVAL 15 MINUTES
WHERE username = "alice@example.com"
  AND failed_attempts >= 5

-- Change password (update hash and salt)
UPDATE credentials
SET hash = @new_pqc_encrypted_hash,
    salt = @new_random_salt,
    iterations = 250000000,
    updated_at = NOW(),
    failed_attempts = 0
WHERE username = "alice@example.com"
```

**Delete Credential:**
```knirvql
-- Soft delete (tombstone)
DELETE FROM credentials
WHERE username = "alice@example.com"

-- Permanent delete (hard delete, requires admin privilege)
DELETE FROM credentials
WHERE username = "alice@example.com"
PERMANENT
```

**Aggregations:**
```knirvql
-- Count total active users
COUNT CREDENTIAL FROM credentials
WHERE status = "active"

-- Count users by status
COUNT CREDENTIAL FROM credentials
GROUP BY status

-- Count admin users
COUNT CREDENTIAL FROM credentials
WHERE metadata.role = "admin"

-- Average failed attempts
AVG failed_attempts FROM credentials
WHERE status = "active"
```

**Field Specifications:**

| Field | Type | Size | Encrypted | Indexed | Notes |
|-------|------|------|-----------|---------|-------|
| `hash` | BYTEA | ~1200 bytes | PQC | No | Kyber-768 ciphertext (~1088) + AES-GCM overhead |
| `salt` | BYTEA | 32 bytes | No | No | Cryptographically random, unique per credential |
| `iterations` | INTEGER | 4 bytes | No | No | Adaptive: 100M (low), 250M (medium), 500M (high) |
| `metadata` | JSONB | Variable | No | GIN | Stores tags, department, custom fields |

---

### 3.2 PQC Keys Collection

**Purpose:** Store post-quantum cryptographic key pairs

#### Collection Creation & Schema

```knirvql
-- Create PQC keys collection
CREATE COLLECTION pqc_keys WITH SCHEMA {
    "entryType": "PQC_KEY",
    "fields": {
        "key_id": {
            "type": "uuid",
            "required": true,
            "unique": true,
            "autoGenerate": true
        },
        "key_name": {
            "type": "string",
            "required": true,
            "maxLength": 100
        },
        "key_purpose": {
            "type": "string",
            "required": true,
            "enum": ["encryption", "signature", "kex"]
        },
        "kyber_public_key": {
            "type": "binary",
            "required": true,
            "minLength": 1184,
            "maxLength": 1184
        },
        "kyber_private_key": {
            "type": "binary",
            "required": true,
            "encrypted": true,
            "minLength": 2400,
            "maxLength": 2400
        },
        "dilithium_public_key": {
            "type": "binary",
            "minLength": 1952,
            "maxLength": 1952
        },
        "dilithium_private_key": {
            "type": "binary",
            "encrypted": true,
            "minLength": 4000,
            "maxLength": 4000
        },
        "algorithm": {
            "type": "string",
            "required": true,
            "enum": ["Kyber-768", "Kyber-1024", "Dilithium-3", "Dilithium-5"]
        },
        "created_at": {
            "type": "timestamp",
            "required": true,
            "autoGenerate": true
        },
        "expires_at": {
            "type": "timestamp"
        },
        "rotated_at": {
            "type": "timestamp"
        },
        "status": {
            "type": "string",
            "required": true,
            "default": "active",
            "enum": ["active", "rotated", "revoked", "expired"]
        },
        "kdf_salt": {
            "type": "binary",
            "required": true,
            "minLength": 32,
            "maxLength": 32
        },
        "kdf_iterations": {
            "type": "integer",
            "required": true,
            "default": 100000,
            "min": 10000
        }
    }
}

-- Create indexes
CREATE INDEX pqc_keys:key_id ON pqc_keys (key_id) UNIQUE
CREATE INDEX pqc_keys:status ON pqc_keys (status) WHERE status = "active"
CREATE INDEX pqc_keys:expires_at ON pqc_keys (expires_at)
CREATE INDEX pqc_keys:key_purpose ON pqc_keys (key_purpose)
```

#### KNIRVQL Operations

**Generate and Store PQC Key Pair:**
```knirvql
-- Insert new PQC key pair
INSERT INTO pqc_keys
SET key_name = "master-encryption-key",
    key_purpose = "encryption",
    kyber_public_key = @kyber_public_key,
    kyber_private_key = @kyber_private_key_encrypted,
    dilithium_public_key = @dilithium_public_key,
    dilithium_private_key = @dilithium_private_key_encrypted,
    algorithm = "Kyber-768",
    kdf_salt = @kdf_salt,
    kdf_iterations = 100000,
    expires_at = NOW() + INTERVAL 365 DAYS,
    status = "active"
RETURNING key_id, created_at
```

**Retrieve Active Key:**
```knirvql
-- Get active encryption key
GET PQC_KEY FROM pqc_keys
WHERE key_purpose = "encryption"
  AND status = "active"
  AND (expires_at IS NULL OR expires_at > NOW())
ORDER BY created_at DESC
LIMIT 1
```

**Key Rotation:**
```knirvql
-- Mark old key as rotated
UPDATE pqc_keys
SET status = "rotated",
    rotated_at = NOW()
WHERE key_id = @old_key_id

-- Insert new key
INSERT INTO pqc_keys
SET key_name = "master-encryption-key-v2",
    key_purpose = "encryption",
    kyber_public_key = @new_kyber_public_key,
    kyber_private_key = @new_kyber_private_key_encrypted,
    algorithm = "Kyber-768",
    kdf_salt = @new_kdf_salt,
    kdf_iterations = 100000,
    status = "active"
```

**Revoke Compromised Key:**
```knirvql
UPDATE pqc_keys
SET status = "revoked"
WHERE key_id = @compromised_key_id
```

---

### 3.3 Sessions Table

**Purpose:** Track active authentication sessions

```sql
CREATE TABLE sessions (
    -- Primary Key
    id                  BIGSERIAL PRIMARY KEY,

    -- Session Identity
    session_id          UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    credential_id       BIGINT NOT NULL REFERENCES credentials(id) ON DELETE CASCADE,

    -- Session Data
    token_hash          BYTEA NOT NULL,              -- SHA-256 hash of session token
    ip_address          INET NOT NULL,
    user_agent          TEXT,

    -- Timestamps
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_activity       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at          TIMESTAMP WITH TIME ZONE NOT NULL,

    -- Status
    status              VARCHAR(20) NOT NULL DEFAULT 'active',
    revoked_at          TIMESTAMP WITH TIME ZONE,
    revoke_reason       TEXT,

    CONSTRAINT chk_sessions_status CHECK (status IN ('active', 'expired', 'revoked'))
);

CREATE UNIQUE INDEX idx_sessions_session_id ON sessions(session_id);
CREATE INDEX idx_sessions_credential_id ON sessions(credential_id);
CREATE INDEX idx_sessions_status ON sessions(status) WHERE status = 'active';
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX idx_sessions_token_hash ON sessions(token_hash);
```

---

### 3.4 Metadata Table

**Purpose:** System-wide configuration and state

```sql
CREATE TABLE metadata (
    -- Primary Key
    key                 VARCHAR(100) PRIMARY KEY,

    -- Value
    value               JSONB NOT NULL,
    value_type          VARCHAR(20) NOT NULL,        -- string, integer, boolean, json

    -- Metadata
    description         TEXT,
    category            VARCHAR(50),                 -- system, security, performance

    -- Timestamps
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_metadata_value_type CHECK (value_type IN ('string', 'integer', 'boolean', 'json'))
);

CREATE INDEX idx_metadata_category ON metadata(category);
```

**Example Entries:**
```sql
INSERT INTO metadata (key, value, value_type, description, category) VALUES
('master_key_id', '"550e8400-e29b-41d4-a716-446655440000"', 'string', 'Active master key ID', 'security'),
('default_kdf_iterations', '100000000', 'integer', 'Default KDF iterations', 'security'),
('max_failed_attempts', '5', 'integer', 'Max failed auth attempts before lock', 'security'),
('session_timeout_minutes', '60', 'integer', 'Session expiration time', 'security'),
('enable_audit_log', 'true', 'boolean', 'Enable audit logging', 'security');
```

---

## 4. Monitoring Tables

### 4.1 Device Status Table

**Purpose:** Track ASIC hardware health and performance

```sql
CREATE TABLE device_status (
    -- Primary Key
    id                  BIGSERIAL PRIMARY KEY,

    -- Device Identity
    device_id           VARCHAR(50) NOT NULL,        -- USB device ID or serial
    device_type         VARCHAR(50) NOT NULL DEFAULT 'Antminer-S3',

    -- Hardware Status
    chain_num           INTEGER NOT NULL,            -- Number of ASIC chains (8)
    asic_num            INTEGER NOT NULL,            -- ASICs per chain (32)
    hash_rate_ghps      DECIMAL(10,2) NOT NULL,      -- Current hash rate (GH/s)

    -- Temperature Monitoring
    temperatures        JSONB NOT NULL,              -- Array of temp readings per chain
    temp_avg            DECIMAL(5,2),                -- Average temperature (°C)
    temp_max            DECIMAL(5,2),                -- Maximum temperature (°C)

    -- Fan Status
    fan_speeds          JSONB NOT NULL,              -- Fan speed readings (RPM)
    fan_pwm             INTEGER,                     -- PWM duty cycle (0-100)

    -- Power and Voltage
    voltage             DECIMAL(5,3),                -- Operating voltage (V)
    frequency_mhz       INTEGER,                     -- ASIC frequency (MHz)
    power_watts         DECIMAL(8,2),                -- Estimated power consumption

    -- Error Tracking
    nonce_errors        BIGINT NOT NULL DEFAULT 0,
    hardware_errors     BIGINT NOT NULL DEFAULT 0,
    communication_errors BIGINT NOT NULL DEFAULT 0,

    -- Timestamps
    timestamp           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Status
    status              VARCHAR(20) NOT NULL DEFAULT 'online',

    CONSTRAINT chk_device_status_status CHECK (status IN ('online', 'offline', 'degraded', 'error'))
);

CREATE INDEX idx_device_status_device_id ON device_status(device_id);
CREATE INDEX idx_device_status_timestamp ON device_status(timestamp DESC);
CREATE INDEX idx_device_status_status ON device_status(status);

-- Retention policy: Keep detailed data for 7 days, aggregate older data
CREATE INDEX idx_device_status_retention ON device_status(timestamp)
    WHERE timestamp < NOW() - INTERVAL '7 days';
```

---

### 4.2 Performance Metrics Table

**Purpose:** Track KDF performance and authentication latency

```sql
CREATE TABLE performance_metrics (
    -- Primary Key
    id                  BIGSERIAL PRIMARY KEY,

    -- Metric Identity
    metric_type         VARCHAR(50) NOT NULL,        -- kdf_duration, auth_latency, etc.
    operation           VARCHAR(50) NOT NULL,        -- store, verify

    -- KDF Metrics
    iterations          INTEGER,                     -- KDF iteration count
    kdf_duration_ms     INTEGER,                     -- KDF computation time (ms)
    device_used         VARCHAR(50),                 -- asic, cpu

    -- Authentication Metrics
    auth_latency_ms     INTEGER,                     -- Total authentication time (ms)

    -- Component Breakdown
    db_query_ms         INTEGER,                     -- Database query time
    pqc_decrypt_ms      INTEGER,                     -- PQC decryption time
    kdf_compute_ms      INTEGER,                     -- KDF computation time
    comparison_ms       INTEGER,                     -- Hash comparison time

    -- Resource Usage
    cpu_percent         DECIMAL(5,2),
    memory_mb           INTEGER,

    -- Timestamps
    timestamp           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Result
    success             BOOLEAN NOT NULL
);

CREATE INDEX idx_performance_metrics_metric_type ON performance_metrics(metric_type);
CREATE INDEX idx_performance_metrics_timestamp ON performance_metrics(timestamp DESC);
CREATE INDEX idx_performance_metrics_operation ON performance_metrics(operation);

-- Retention: Aggregate hourly after 24 hours, delete raw data after 30 days
```

---

### 4.3 Health Checks Table

**Purpose:** System health check results

```sql
CREATE TABLE health_checks (
    -- Primary Key
    id                  BIGSERIAL PRIMARY KEY,

    -- Check Identity
    check_name          VARCHAR(100) NOT NULL,
    check_type          VARCHAR(50) NOT NULL,        -- system, database, asic, network

    -- Results
    status              VARCHAR(20) NOT NULL,        -- healthy, degraded, unhealthy
    response_time_ms    INTEGER,

    -- Details
    details             JSONB DEFAULT '{}',
    error_message       TEXT,

    -- Timestamps
    timestamp           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_health_checks_status CHECK (status IN ('healthy', 'degraded', 'unhealthy'))
);

CREATE INDEX idx_health_checks_timestamp ON health_checks(timestamp DESC);
CREATE INDEX idx_health_checks_status ON health_checks(status);
CREATE INDEX idx_health_checks_check_name ON health_checks(check_name);
```

---

## 5. Security Tables

### 5.1 Audit Log Table

**Purpose:** Immutable security event log

```sql
CREATE TABLE audit_log (
    -- Primary Key
    id                  BIGSERIAL PRIMARY KEY,

    -- Event Identity
    event_id            UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    event_type          VARCHAR(50) NOT NULL,        -- credential_stored, credential_verified, etc.
    event_category      VARCHAR(50) NOT NULL,        -- authentication, authorization, configuration

    -- Subject
    username            VARCHAR(255),
    credential_id       BIGINT REFERENCES credentials(id) ON DELETE SET NULL,

    -- Action Details
    action              VARCHAR(100) NOT NULL,
    resource            VARCHAR(255),
    result              VARCHAR(20) NOT NULL,        -- success, failure, error

    -- Context
    ip_address          INET,
    user_agent          TEXT,
    session_id          UUID,

    -- Event Data
    details             JSONB DEFAULT '{}',
    error_message       TEXT,

    -- Timestamps
    timestamp           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_audit_log_result CHECK (result IN ('success', 'failure', 'error'))
);

-- Immutable: Prevent updates and deletes
CREATE RULE audit_log_no_update AS ON UPDATE TO audit_log DO INSTEAD NOTHING;
CREATE RULE audit_log_no_delete AS ON DELETE TO audit_log DO INSTEAD NOTHING;

CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp DESC);
CREATE INDEX idx_audit_log_username ON audit_log(username);
CREATE INDEX idx_audit_log_event_type ON audit_log(event_type);
CREATE INDEX idx_audit_log_result ON audit_log(result) WHERE result != 'success';
CREATE INDEX idx_audit_log_ip_address ON audit_log(ip_address);
CREATE INDEX idx_audit_log_details ON audit_log USING GIN(details);

-- Partition by month for performance
CREATE TABLE audit_log_y2025m01 PARTITION OF audit_log
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');
```

**Event Types:**
- `credential_stored` - New credential created
- `credential_verified` - Authentication attempt
- `credential_updated` - Password changed
- `credential_deleted` - Credential removed
- `session_created` - New session started
- `session_revoked` - Session terminated
- `config_changed` - System configuration modified
- `key_rotated` - Cryptographic key rotation
- `access_denied` - Authorization failure

---

### 5.2 Rate Limits Table

**Purpose:** Track authentication attempts for rate limiting

```sql
CREATE TABLE rate_limits (
    -- Primary Key
    id                  BIGSERIAL PRIMARY KEY,

    -- Identity
    identifier          VARCHAR(255) NOT NULL,       -- username, IP, or combined
    limit_type          VARCHAR(50) NOT NULL,        -- user, ip, global

    -- Counters
    attempt_count       INTEGER NOT NULL DEFAULT 1,
    success_count       INTEGER NOT NULL DEFAULT 0,
    failure_count       INTEGER NOT NULL DEFAULT 0,

    -- Time Windows
    window_start        TIMESTAMP WITH TIME ZONE NOT NULL,
    window_end          TIMESTAMP WITH TIME ZONE NOT NULL,

    -- Status
    blocked_until       TIMESTAMP WITH TIME ZONE,

    -- Timestamps
    first_attempt       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_attempt        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_rate_limits_limit_type CHECK (limit_type IN ('user', 'ip', 'global'))
);

CREATE UNIQUE INDEX idx_rate_limits_identifier_type ON rate_limits(identifier, limit_type);
CREATE INDEX idx_rate_limits_blocked_until ON rate_limits(blocked_until) WHERE blocked_until IS NOT NULL;
CREATE INDEX idx_rate_limits_window_end ON rate_limits(window_end);

-- Cleanup old entries (TTL: 1 hour after window_end)
CREATE INDEX idx_rate_limits_cleanup ON rate_limits(window_end)
    WHERE window_end < NOW() - INTERVAL '1 hour';
```

---

### 5.3 Threat Events Table

**Purpose:** Track security incidents and anomalies

```sql
CREATE TABLE threat_events (
    -- Primary Key
    id                  BIGSERIAL PRIMARY KEY,

    -- Event Identity
    event_id            UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    threat_type         VARCHAR(50) NOT NULL,        -- brute_force, credential_stuffing, etc.
    severity            VARCHAR(20) NOT NULL,        -- low, medium, high, critical

    -- Subject
    username            VARCHAR(255),
    ip_address          INET,

    -- Detection
    detected_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    detection_method    VARCHAR(100),                -- rate_limit, pattern_match, ml_model

    -- Details
    indicators          JSONB NOT NULL,              -- Attack indicators and evidence
    confidence_score    DECIMAL(5,4),                -- 0.0000 to 1.0000

    -- Response
    action_taken        VARCHAR(50),                 -- blocked, alerted, logged
    resolved_at         TIMESTAMP WITH TIME ZONE,
    resolution          TEXT,

    -- Status
    status              VARCHAR(20) NOT NULL DEFAULT 'open',

    CONSTRAINT chk_threat_events_severity CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    CONSTRAINT chk_threat_events_status CHECK (status IN ('open', 'investigating', 'resolved', 'false_positive'))
);

CREATE INDEX idx_threat_events_detected_at ON threat_events(detected_at DESC);
CREATE INDEX idx_threat_events_severity ON threat_events(severity);
CREATE INDEX idx_threat_events_status ON threat_events(status) WHERE status IN ('open', 'investigating');
CREATE INDEX idx_threat_events_ip_address ON threat_events(ip_address);
CREATE INDEX idx_threat_events_indicators ON threat_events USING GIN(indicators);
```

---

### 5.4 Access Control Table

**Purpose:** Role-based access control (RBAC)

```sql
CREATE TABLE access_control (
    -- Primary Key
    id                  BIGSERIAL PRIMARY KEY,

    -- Subject
    credential_id       BIGINT NOT NULL REFERENCES credentials(id) ON DELETE CASCADE,

    -- Role
    role                VARCHAR(50) NOT NULL,        -- admin, operator, auditor, user

    -- Permissions (JSON array of permission strings)
    permissions         JSONB NOT NULL DEFAULT '[]',

    -- Scope
    resource_pattern    VARCHAR(255),                -- Resource pattern (e.g., /api/v1/credentials/*)

    -- Timestamps
    granted_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at          TIMESTAMP WITH TIME ZONE,
    revoked_at          TIMESTAMP WITH TIME ZONE,

    -- Status
    status              VARCHAR(20) NOT NULL DEFAULT 'active',

    CONSTRAINT chk_access_control_role CHECK (role IN ('admin', 'operator', 'auditor', 'user')),
    CONSTRAINT chk_access_control_status CHECK (status IN ('active', 'expired', 'revoked'))
);

CREATE INDEX idx_access_control_credential_id ON access_control(credential_id);
CREATE INDEX idx_access_control_role ON access_control(role);
CREATE INDEX idx_access_control_status ON access_control(status) WHERE status = 'active';
CREATE INDEX idx_access_control_permissions ON access_control USING GIN(permissions);
```

---

## 6. Indexing Strategy

### 6.1 Index Types

**B-Tree Indexes (Default):**
- Primary keys
- Foreign keys
- Equality and range queries
- Sorting operations

**GIN Indexes (Generalized Inverted Index):**
- JSONB columns
- Full-text search
- Array containment queries

**Partial Indexes:**
- Active records only (`WHERE status = 'active'`)
- Recent data (`WHERE timestamp > NOW() - INTERVAL '7 days'`)
- Error conditions (`WHERE result != 'success'`)

### 6.2 Query Performance Targets

| Operation | Target Latency | Index Strategy |
|-----------|----------------|----------------|
| Credential lookup by username | < 5ms | Unique index on username |
| Authentication verification | < 500ms | Composite index on username + status |
| Audit log search (24h) | < 100ms | Partitioned index on timestamp |
| Session validation | < 10ms | Unique index on session_id |
| Device status (latest) | < 20ms | Index on (device_id, timestamp DESC) |

---

## 7. Encryption Strategy

### 7.1 Data Classification

**Level 1: Critical (PQC Encrypted):**
- `credentials.hash` - KDF outputs
- `pqc_keys.kyber_private_key` - Private keys
- `pqc_keys.dilithium_private_key` - Private keys

**Level 2: Sensitive (Database Encryption):**
- `sessions.token_hash` - Session tokens
- `audit_log.details` - Event details
- `threat_events.indicators` - Attack data

**Level 3: Metadata (Unencrypted):**
- Usernames, timestamps, status fields
- Performance metrics
- Configuration data

### 7.2 Encryption Implementation

**PQC Encryption (Application Layer):**
```
Plaintext (32 bytes)
    ↓
AES-256-GCM encryption (key from Kyber KEM)
    ↓
Kyber-768 encapsulation
    ↓
Ciphertext (~1200 bytes) → Store in database
```

**Database-Level Encryption (PostgreSQL):**
```ini
# postgresql.conf
ssl = on
ssl_cert_file = '/etc/ssl/certs/server.crt'
ssl_key_file = '/etc/ssl/private/server.key'

# Enable transparent data encryption (TDE) via pgcrypto
CREATE EXTENSION pgcrypto;
```

**At-Rest Encryption:**
- Full disk encryption (LUKS on Linux)
- Encrypted PostgreSQL tablespaces
- Encrypted backup files

---

## 8. Backup and Recovery

### 8.1 Backup Strategy

**Backup Types:**

1. **Full Backup (Daily)**
   - Complete database dump
   - Includes all schemas and data
   - Retention: 30 days

2. **Incremental Backup (Hourly)**
   - WAL archive shipping
   - Point-in-time recovery capability
   - Retention: 7 days

3. **Snapshot Backup (On-demand)**
   - Before major changes
   - Before key rotation
   - Indefinite retention for compliance

**PostgreSQL Backup Commands:**
```bash
# Full backup
pg_dump -Fc -Z9 -f /backups/asic_shield_$(date +%Y%m%d_%H%M%S).dump asic_shield

# WAL archiving
archive_command = 'rsync -a %p backup-server:/wal_archive/%f'

# Restore
pg_restore -d asic_shield /backups/asic_shield_20250124_120000.dump
```

**SQLite Backup:**
```bash
# Online backup (WAL mode)
sqlite3 /data/vault.db ".backup /backups/vault_$(date +%Y%m%d_%H%M%S).db"
```

### 8.2 Backup Verification

```sql
CREATE TABLE backup_log (
    id                  BIGSERIAL PRIMARY KEY,
    backup_type         VARCHAR(20) NOT NULL,        -- full, incremental, snapshot
    backup_path         TEXT NOT NULL,
    file_size_bytes     BIGINT NOT NULL,
    checksum_sha256     VARCHAR(64) NOT NULL,
    started_at          TIMESTAMP WITH TIME ZONE NOT NULL,
    completed_at        TIMESTAMP WITH TIME ZONE NOT NULL,
    verified_at         TIMESTAMP WITH TIME ZONE,
    status              VARCHAR(20) NOT NULL,        -- success, failed, corrupted

    CONSTRAINT chk_backup_log_type CHECK (backup_type IN ('full', 'incremental', 'snapshot')),
    CONSTRAINT chk_backup_log_status CHECK (status IN ('success', 'failed', 'corrupted'))
);

CREATE INDEX idx_backup_log_completed_at ON backup_log(completed_at DESC);
```

---

## 9. Migration Strategy

### 9.1 Schema Versioning

```sql
CREATE TABLE schema_migrations (
    id                  BIGSERIAL PRIMARY KEY,
    version             VARCHAR(20) NOT NULL UNIQUE,
    description         TEXT NOT NULL,
    applied_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    checksum            VARCHAR(64) NOT NULL
);

CREATE INDEX idx_schema_migrations_version ON schema_migrations(version);
```

**Migration Process:**
1. Version control all schema changes
2. Test migrations on staging database
3. Create rollback scripts for each migration
4. Apply migrations during maintenance window
5. Verify data integrity post-migration

### 9.2 SQLite to PostgreSQL Migration

**Migration Steps:**

1. **Export Data from SQLite:**
```bash
sqlite3 vault.db .dump > dump.sql
```

2. **Convert SQL Dialect:**
```python
# Convert SQLite types to PostgreSQL
# INTEGER → BIGSERIAL
# TEXT → VARCHAR or TEXT
# BLOB → BYTEA
# REAL → DECIMAL
```

3. **Import to PostgreSQL:**
```bash
psql -d asic_shield -f converted_dump.sql
```

4. **Verify Data Integrity:**
```sql
SELECT COUNT(*) FROM credentials;
SELECT pg_size_pretty(pg_database_size('asic_shield'));
```

---

## 10. Performance Optimization

### 10.1 Query Optimization

**Credential Verification (Hot Path):**
```sql
-- Optimized query with covering index
EXPLAIN ANALYZE
SELECT hash, salt, iterations, pqc_key_id
FROM credentials
WHERE username = 'alice@example.com' AND status = 'active';

-- Query plan should show: Index Scan using idx_credentials_username
```

**Audit Log Search:**
```sql
-- Partition pruning for time-range queries
EXPLAIN ANALYZE
SELECT event_type, username, timestamp, details
FROM audit_log
WHERE timestamp >= NOW() - INTERVAL '24 hours'
  AND event_type = 'credential_verified';

-- Should use partition-wise scan on current month partition
```

### 10.2 Connection Pooling

**PgBouncer Configuration:**
```ini
[databases]
asic_shield = host=localhost port=5432 dbname=asic_shield

[pgbouncer]
pool_mode = transaction
max_client_conn = 1000
default_pool_size = 50
reserve_pool_size = 10
```

### 10.3 Monitoring Queries

**Slow Query Detection:**
```sql
SELECT query, calls, total_time, mean_time
FROM pg_stat_statements
ORDER BY mean_time DESC
LIMIT 10;
```

**Index Usage:**
```sql
SELECT schemaname, tablename, indexname, idx_scan, idx_tup_read, idx_tup_fetch
FROM pg_stat_user_indexes
WHERE idx_scan = 0;
```

---

## Appendix A: Sample Data

### A.1 Credential Record

```sql
INSERT INTO credentials (
    username, display_name, email,
    hash, salt, iterations, algorithm,
    pqc_algorithm, pqc_key_id,
    metadata, status
) VALUES (
    'alice@example.com',
    'Alice Johnson',
    'alice@example.com',
    decode('a1b2c3...', 'hex'),  -- 1200-byte PQC-encrypted hash
    decode('d4e5f6...', 'hex'),  -- 32-byte salt
    100000000,
    'PBKDF2-SHA256-ASIC',
    'Kyber-768',
    1,
    '{"department": "engineering", "role": "admin"}',
    'active'
);
```

### A.2 Audit Log Entry

```sql
INSERT INTO audit_log (
    event_type, event_category,
    username, action, result,
    ip_address, details
) VALUES (
    'credential_verified',
    'authentication',
    'alice@example.com',
    'verify_password',
    'success',
    '192.168.1.100',
    '{"iterations": 100000000, "kdf_duration_ms": 215, "device": "asic"}'::jsonb
);
```

---

## Appendix B: Maintenance Scripts

### B.1 Cleanup Script

```sql
-- Delete expired sessions
DELETE FROM sessions
WHERE status = 'active' AND expires_at < NOW();

-- Delete old rate limit entries
DELETE FROM rate_limits
WHERE window_end < NOW() - INTERVAL '1 hour';

-- Archive old audit logs (>90 days)
INSERT INTO audit_log_archive
SELECT * FROM audit_log
WHERE timestamp < NOW() - INTERVAL '90 days';

DELETE FROM audit_log
WHERE timestamp < NOW() - INTERVAL '90 days';

-- Vacuum and analyze
VACUUM ANALYZE credentials;
VACUUM ANALYZE audit_log;
```

### B.2 Health Check Script

```sql
-- Check table sizes
SELECT
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;

-- Check index bloat
SELECT
    schemaname,
    tablename,
    indexname,
    pg_size_pretty(pg_relation_size(indexrelid)) AS index_size
FROM pg_stat_user_indexes
ORDER BY pg_relation_size(indexrelid) DESC;
```

---

## Document Version History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2024-12-24 | Hasher Team | Initial database specification |

---

**Status:** Ready for Implementation
**Next Review:** After Phase 1 PoC completion
**Approval Required:** Architecture team, Security team

