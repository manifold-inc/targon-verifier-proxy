package routes

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"api/internal/shared"

	"github.com/aidarkhanov/nanoid"
	"github.com/labstack/echo/v4"
)

// checkAdminAuth validates that the request has a valid admin API key
func checkAdminAuth(c echo.Context) (bool, int, string) {
	cc := c.(*shared.Context)

	// Check admin authorization from Bearer token
	authHeader := c.Request().Header.Get("Authorization")
	if authHeader == "" {
		cc.Log.Warn("Missing Authorization header")
		return false, http.StatusUnauthorized, "Authorization required"
	}

	// Parse Bearer token
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		cc.Log.Warnw("Invalid Authorization format", "header", authHeader)
		return false, http.StatusUnauthorized, "Invalid authorization format. Use 'Bearer YOUR_API_KEY'"
	}

	apiKey := parts[1]

	// Verify the API key is an admin key
	var isAdmin bool
	err := cc.Cfg.SqlClient.QueryRow(
		"SELECT is_admin FROM api_keys WHERE key_value = ?",
		apiKey,
	).Scan(&isAdmin)

	if err == sql.ErrNoRows {
		cc.Log.Warnw("Invalid API key used for admin operation", "key", apiKey)
		return false, http.StatusUnauthorized, "Invalid API key"
	} else if err != nil {
		cc.Log.Errorw("Database error checking API key", "error", err.Error())
		return false, http.StatusInternalServerError, "Internal server error"
	}

	if !isAdmin {
		cc.Log.Warnw("Non-admin API key used for admin operation")
		return false, http.StatusForbidden, "Administrator privileges required"
	}

	return true, 0, ""
}

// AddKey handler for adding a new API key
func AddKey(c echo.Context) error {
	cc := c.(*shared.Context)
	defer cc.Log.Sync()

	// Check admin authorization
	if isAdmin, code, errMsg := checkAdminAuth(c); !isAdmin {
		return c.JSON(code, map[string]string{"error": errMsg})
	}

	var req shared.AddKeyRequest
	if err := c.Bind(&req); err != nil {
		cc.Log.Errorw("Failed to parse request", "error", err.Error())
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	if req.Hotkey == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "hotkey is required",
		})
	}

	// Generate API key value
	keyValue, err := nanoid.Generate("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", 32)
	if err != nil {
		cc.Log.Errorw("Failed to generate API key", "error", err.Error())
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to generate API key",
		})
	}

	var count int
	err = cc.Cfg.SqlClient.QueryRow("SELECT COUNT(*) FROM api_keys WHERE hotkey = ?", req.Hotkey).Scan(&count)
	if err != nil {
		cc.Log.Errorw("Failed to check for existing hotkey", "error", err.Error())
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to check for existing hotkey",
		})
	}

	if count > 0 {
		cc.Log.Warnw("Attempted to create duplicate hotkey", "hotkey", req.Hotkey)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Hotkey already exists. Use a different hotkey or remove the existing one first.",
		})
	}

	_, err = cc.Cfg.SqlClient.Exec(
		"INSERT INTO api_keys (hotkey, key_value, is_admin) VALUES (?, ?, false)",
		req.Hotkey, keyValue,
	)
	if err != nil {
		cc.Log.Errorw("Failed to insert API key", "error", err.Error())
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to store API key",
		})
	}
	cc.Log.Infow("API key created", "hotkey", req.Hotkey)

	// Return the new key
	return c.JSON(http.StatusOK, shared.ApiKey{
		Hotkey:    req.Hotkey,
		KeyValue:  keyValue,
		CreatedAt: time.Now(),
		IsAdmin:   false, // Always false for newly created keys
	})
}

// RemoveKey handler for removing an API key
func RemoveKey(c echo.Context) error {
	cc := c.(*shared.Context)
	defer cc.Log.Sync()

	// Check admin authorization
	if isAdmin, code, errMsg := checkAdminAuth(c); !isAdmin {
		return c.JSON(code, map[string]string{"error": errMsg})
	}

	// Parse request body
	var req shared.RemoveKeyRequest
	if err := c.Bind(&req); err != nil {
		cc.Log.Errorw("Failed to parse request", "error", err.Error())
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	// Validate required fields
	if req.Hotkey == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "hotkey is required",
		})
	}

	// Delete the key from the database
	result, err := cc.Cfg.SqlClient.Exec("DELETE FROM api_keys WHERE hotkey = ?", req.Hotkey)
	if err != nil {
		cc.Log.Errorw("Failed to delete API key", "error", err.Error())
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to delete API key",
		})
	}

	// Check if any rows were affected
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		cc.Log.Errorw("Failed to get rows affected", "error", err.Error())
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to confirm deletion",
		})
	}

	if rowsAffected == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "API key not found",
		})
	}

	cc.Log.Infow("API key removed", "hotkey", req.Hotkey)

	return c.JSON(http.StatusOK, map[string]string{
		"message": "API key removed successfully",
	})
}

// GetKey handler for retrieving an API key by hotkey
func GetKey(c echo.Context) error {
	cc := c.(*shared.Context)
	defer cc.Log.Sync()

	// Check admin authorization
	if isAdmin, code, errMsg := checkAdminAuth(c); !isAdmin {
		return c.JSON(code, map[string]string{"error": errMsg})
	}

	// Parse request body
	var req shared.GetKeyRequest
	if err := c.Bind(&req); err != nil {
		cc.Log.Errorw("Failed to parse request", "error", err.Error())
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	// Validate required fields
	if req.Hotkey == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "hotkey is required",
		})
	}

	// Query for the API key
	var keyValue string
	err := cc.Cfg.SqlClient.QueryRow(
		"SELECT key_value FROM api_keys WHERE hotkey = ?",
		req.Hotkey,
	).Scan(&keyValue)

	if err == sql.ErrNoRows {
		cc.Log.Warnw("API key not found", "hotkey", req.Hotkey)
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "API key not found",
		})
	} else if err != nil {
		cc.Log.Errorw("Database error retrieving API key", "error", err.Error(), "hotkey", req.Hotkey)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve API key",
		})
	}

	cc.Log.Infow("API key retrieved", "hotkey", req.Hotkey)

	// Return only the key_value and hotkey
	return c.JSON(http.StatusOK, map[string]string{
		"hotkey":    req.Hotkey,
		"key_value": keyValue,
	})
}
