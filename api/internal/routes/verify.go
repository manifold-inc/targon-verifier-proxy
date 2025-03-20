package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"api/internal/shared"

	"github.com/labstack/echo/v4"
)

func Verify(c echo.Context) error {
	cc := c.(*shared.Context)
	startTime := time.Now()

	var request shared.VerificationRequest
	if err := c.Bind(&request); err != nil {
		cc.Log.Errorw("Failed to parse request", "error", err.Error())
		return c.JSON(http.StatusBadRequest, map[string]any{
			"verified": false,
			"error":    "Invalid request format",
		})
	}

	// Validate required fields
	if missingField, isMissing := validateRequiredFields(cc, &request); isMissing {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"verified": false,
			"error":    "Missing required field: " + missingField,
		})
	}

	valid, err := validateAPIKey(cc)
	if !valid {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"verified": false,
			"error":    err.Error(),
		})
	}

	cc.Log.Infow("Verification request received",
		"model", request.Model,
		"request_type", request.RequestType,
		"request_id", request.RequestID,
	)

	if request.RequestID != "" {
		if cachedResponse, found := cc.Cfg.Cache.Get(request.RequestID); found {
			var response shared.VerificationResponse
			if err := json.Unmarshal(cachedResponse, &response); err != nil {
				cc.Log.Warnw("Failed to unmarshal cached response", "error", err.Error(), "request_id", request.RequestID)
			} else {
				cc.Log.Infow("Cache hit",
					"request_id", request.RequestID,
					"duration_ms", time.Since(startTime).Milliseconds(),
				)

				// Log cached verification result
				cc.Log.Infow("Cached verification result",
					"request_id", request.RequestID,
					"verified", response.Verified,
					"model", request.Model,
					"error", response.Error,
					"cause", response.Cause,
				)

				return c.JSON(http.StatusOK, response)
			}
		}
	}

	response, err := forwardToValis(cc, &request)
	if err != nil {
		cc.Log.Errorw("Verification failed", "error", err.Error(), "request_id", request.RequestID)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"verified": false,
			"error":    "Verification service error: " + err.Error(),
		})
	}

	if request.RequestID != "" && response != nil {
		cc.Cfg.Cache.Set(request.RequestID, response, 72*time.Minute)
		cc.Log.Infow("Cached response", "request_id", request.RequestID)
	}

	cc.Log.Infow("Verification completed",
		"request_id", request.RequestID,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return c.JSONBlob(http.StatusOK, response)
}

// validateRequiredFields checks if all required fields are present in the request
func validateRequiredFields(cc *shared.Context, request *shared.VerificationRequest) (string, bool) {
	if request.Model == "" {
		cc.Log.Warnw("Missing required field: model")
		return "model", true
	}

	if request.RequestType == "" {
		cc.Log.Warnw("Missing required field: request_type")
		return "request_type", true
	}

	if request.RequestParams == nil {
		cc.Log.Warnw("Missing required field: request_params")
		return "request_params", true
	}

	if request.RawChunks == nil {
		cc.Log.Warnw("Missing required field: raw_chunks")
		return "raw_chunks", true
	}

	return "", false
}

// validateAPIKey checks if the request has a valid API key
func validateAPIKey(cc *shared.Context) (bool, error) {
	authHeader := cc.Request().Header.Get("Authorization")
	if authHeader == "" {
		cc.Log.Warn("Missing Authorization header")
		return false, fmt.Errorf("authorization required")
	}

	// Parse Bearer token
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		cc.Log.Warnw("Invalid Authorization format", "header", authHeader)
		return false, fmt.Errorf("invalid authorization format")
	}

	apiKey := parts[1]

	var hotkey string
	err := cc.Cfg.SqlClient.QueryRow(
		"SELECT hotkey FROM api_keys WHERE key_value = ?",
		apiKey,
	).Scan(&hotkey)
	if err != nil {
		cc.Log.Warnw("Invalid API key", "key", apiKey, "error", err.Error())
		return false, fmt.Errorf("invalid API key")
	}

	_, err = cc.Cfg.SqlClient.Exec(
		"UPDATE api_keys SET last_used_at = ? WHERE hotkey = ?",
		time.Now(), hotkey,
	)
	if err != nil {
		cc.Log.Warnw("Failed to update last_used_at", "error", err.Error(), "hotkey", hotkey)
	}

	return true, nil
}

// forwardToValis sends the verification request to the Valis service
func forwardToValis(cc *shared.Context, req *shared.VerificationRequest) ([]byte, error) {
	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	requestBody, err := json.Marshal(req)
	if err != nil {
		cc.Log.Errorw("Failed to marshal request", "error", err.Error())
		return nil, fmt.Errorf("failed to prepare request: %w", err)
	}

	if cc.Cfg.Env.Debug {
		cc.Log.Debugw("Forwarding verification request",
			"request_id", req.RequestID,
			"model", req.Model,
			"request_type", req.RequestType,
			"chunks_count", len(req.RawChunks),
		)
	}

	backendURL := fmt.Sprintf("%s/verify", cc.Cfg.Env.HaproxyURL)
	httpReq, err := http.NewRequest(http.MethodPost, backendURL, bytes.NewReader(requestBody))
	if err != nil {
		cc.Log.Errorw("Failed to create request", "error", err.Error())
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	switch req.Model {
	case "deepseek-ai/DeepSeek-R1":
		httpReq.Header.Set("x-backend-server", "r1")
	case "deepseek-ai/DeepSeek-V3":
		httpReq.Header.Set("x-backend-server", "v3")
	default:
		cc.Log.Errorw("Unsupported model", "model", req.Model)
		return nil, fmt.Errorf("unsupported model: %s", req.Model)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := client.Do(httpReq)
	if err != nil {
		cc.Log.Errorw("Failed to send request to backend", "error", err.Error(), "url", backendURL)
		return nil, fmt.Errorf("failed to send request to backend: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		cc.Log.Errorw("Failed to read response body", "error", err.Error())
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}
