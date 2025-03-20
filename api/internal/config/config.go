package config

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type CacheEntry struct {
	Response  []byte
	ExpiresAt time.Time
}

type VerificationCache struct {
	cache map[string]CacheEntry
	mutex sync.RWMutex
}

type Environment struct {
	Debug         bool
	HaproxyURL    string
	AdminHotkey   string
	AdminKeyValue string
}

func NewVerificationCache() *VerificationCache {
	return &VerificationCache{
		cache: make(map[string]CacheEntry),
	}
}

func (c *VerificationCache) Set(requestID string, response []byte, ttl time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cache[requestID] = CacheEntry{
		Response:  response,
		ExpiresAt: time.Now().Add(ttl),
	}
}

func (c *VerificationCache) Get(requestID string) ([]byte, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.cache[requestID]
	if !exists {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		go func() {
			c.mutex.Lock()
			delete(c.cache, requestID)
			c.mutex.Unlock()
		}()
		return nil, false
	}

	return entry.Response, true
}

func (c *VerificationCache) Cleanup() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	for key, entry := range c.cache {
		if now.After(entry.ExpiresAt) {
			delete(c.cache, key)
		}
	}
}

func (c *VerificationCache) StartCleanupRoutine(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			c.Cleanup()
		}
	}()
}

type Config struct {
	Env       Environment
	SqlClient *sql.DB
	Cache     *VerificationCache
}

func (c *Config) Shutdown() {
	if c.SqlClient != nil {
		c.SqlClient.Close()
	}
}

func getEnv(env, fallback string) string {
	if value, ok := os.LookupEnv(env); ok {
		return value
	}
	return fallback
}

func InitConfig() (*Config, []error) {
	var errs []error

	mysqlHost := getEnv("MYSQL_HOST", "mysql")
	mysqlUser := getEnv("MYSQL_USER", "admin")
	mysqlPassword := getEnv("MYSQL_PASSWORD", "adminpassword")
	mysqlDatabase := getEnv("MYSQL_DATABASE", "targon_proxy")

	DSN := fmt.Sprintf("%s:%s@tcp(%s:3306)/%s?parseTime=true",
		mysqlUser, mysqlPassword, mysqlHost, mysqlDatabase)

	HAPROXY_URL := getEnv("HAPROXY_URL", "http://haproxy")

	ADMIN_HOTKEY := getEnv("ADMIN_HOTKEY", "admin")
	ADMIN_KEY_VALUE := getEnv("ADMIN_API_KEY", "admin_api_key")

	DEBUG, err := strconv.ParseBool(getEnv("DEBUG", "false"))
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) != 0 {
		return nil, errs
	}

	sqlClient, err := sql.Open("mysql", DSN)
	if err != nil {
		return nil, []error{errors.New("failed initializing sqlClient"), err}
	}

	err = sqlClient.Ping()
	if err != nil {
		return nil, []error{errors.New("failed ping to sql db"), err}
	}

	cache := NewVerificationCache()
	cache.StartCleanupRoutine(5 * time.Minute)

	cfg := &Config{
		Env: Environment{
			Debug:         DEBUG,
			HaproxyURL:    HAPROXY_URL,
			AdminHotkey:   ADMIN_HOTKEY,
			AdminKeyValue: ADMIN_KEY_VALUE,
		},
		SqlClient: sqlClient,
		Cache:     cache,
	}

	if ADMIN_KEY_VALUE != "" {
		if err := ensureAdminKey(cfg); err != nil {
			fmt.Printf("Warning: Failed to setup admin key: %v\n", err)
		}
	}

	return cfg, nil
}

// ensureAdminKey ensures an admin API key exists in the database
func ensureAdminKey(cfg *Config) error {
	var count int
	err := cfg.SqlClient.QueryRow("SELECT COUNT(*) FROM api_keys WHERE hotkey = ?", cfg.Env.AdminHotkey).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check for admin key: %w", err)
	}

	if count == 0 {
		_, err = cfg.SqlClient.Exec(
			"INSERT INTO api_keys (hotkey, key_value, is_admin, created_at) VALUES (?, ?, TRUE, ?)",
			cfg.Env.AdminHotkey, cfg.Env.AdminKeyValue, time.Now(),
		)
		if err != nil {
			return fmt.Errorf("failed to create admin key: %w", err)
		}
		fmt.Printf("Created admin API key with hotkey '%s'\n", cfg.Env.AdminHotkey)
	} else {
		_, err = cfg.SqlClient.Exec(
			"UPDATE api_keys SET key_value = ? WHERE hotkey = ?",
			cfg.Env.AdminKeyValue, cfg.Env.AdminHotkey,
		)
		if err != nil {
			return fmt.Errorf("failed to update admin key: %w", err)
		}
		fmt.Printf("Updated admin API key with hotkey '%s'\n", cfg.Env.AdminHotkey)
	}

	return nil
}
