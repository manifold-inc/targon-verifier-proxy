package shared

import (
	"api/internal/config"
	"errors"
	"fmt"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// Context extends echo.Context with application-specific fields
type Context struct {
	echo.Context
	Log   *zap.SugaredLogger
	Reqid string
	Cfg   *config.Config
}

// RequestError represents a standard API error response
type RequestError struct {
	StatusCode int
	Err        error
}

func (r *RequestError) Error() string {
	return fmt.Sprintf("status %d: err %v", r.StatusCode, r.Err)
}

// Common errors
var (
	ErrUnauthorized      = errors.New("unauthorized")
	ErrInvalidAuth       = errors.New("invalid authentication")
	ErrBadRequest        = errors.New("bad request")
	ErrInternalServer    = errors.New("internal server error")
	ErrResourceNotFound  = errors.New("resource not found")
	ErrInvalidParameters = errors.New("invalid parameters")
)

// RequestInfo contains information about an API request
type RequestInfo struct {
	Body            []byte
	UserId          int
	StartingCredits int64
	Id              string
	Chargeable      bool
	StartTime       time.Time
	Endpoint        string
	Model           string
	Miner           *int
	MinerHost       string
}

// Request is a base struct for API requests
type Request struct {
	MaxTokens uint64 `json:"max_tokens"`
}

// ApiKey represents an API key in the system
type ApiKey struct {
	Hotkey    string    `json:"hotkey"`
	KeyValue  string    `json:"key_value"`
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used,omitempty"`
	IsAdmin   bool      `json:"is_admin"`
}

// AddKeyRequest is used to request a new API key
type AddKeyRequest struct {
	Hotkey string `json:"hotkey" validate:"required"`
}

// RemoveKeyRequest is used to request removal of an API key
type RemoveKeyRequest struct {
	Hotkey string `json:"hotkey" validate:"required"`
}

// VerificationRequest is used for verification requests
type VerificationRequest struct {
	Model         string                   `json:"model"`
	RequestType   string                   `json:"request_type"`
	RequestParams map[string]interface{}   `json:"request_params"`
	RawChunks     []map[string]interface{} `json:"raw_chunks"`
	RequestID     string                   `json:"request_id,omitempty"`
}

// VerificationResponse represents a response from the verification service
type VerificationResponse struct {
	RequestID      string      `json:"request_id,omitempty"`
	Verified       bool        `json:"verified"`
	Error          string      `json:"error,omitempty"`
	Cause          string      `json:"cause,omitempty"`
	InputTokens    interface{} `json:"input_tokens,omitempty"`
	ResponseTokens interface{} `json:"response_tokens,omitempty"`
	GPUs           int         `json:"gpus,omitempty"`
}

// GetKeyRequest is used to request an API key by hotkey
type GetKeyRequest struct {
	Hotkey string `json:"hotkey" validate:"required"`
}
