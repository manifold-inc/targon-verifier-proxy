-- Initial schema for targon-verifier-proxy

-- API keys table
CREATE TABLE IF NOT EXISTS api_keys (
    hotkey VARCHAR(255) PRIMARY KEY,
    key_value VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP NULL,
    is_admin BOOLEAN DEFAULT FALSE
);