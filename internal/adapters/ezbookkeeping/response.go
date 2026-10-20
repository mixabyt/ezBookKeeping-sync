package ezbookkeeping

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// maxResponseBody обмежує, скільки байт відповіді читаємо (захист від величезних тіл).
const maxResponseBody = 1 << 20

// APIError — ezBookkeeping відповів, але не успіхом.
//
// Формат відповідей ezBookkeeping (pkg/utils/api.go):
//   - успіх:   HTTP 200, {"success": true, "result": ...}
//   - помилка: HTTP зі статусом самої помилки (400, 401, 500...),
//     {"success": false, "errorCode": ..., "errorMessage": "...", "path": "..."}
//
// Тому успіх — це лише 2xx І success == true одночасно.
type APIError struct {
	StatusCode int    // HTTP-статус відповіді
	Code       int    // errorCode з тіла (0, якщо тіло не JSON)
	Message    string // errorMessage з тіла або сире тіло
	Path       string
}

func (e *APIError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("ezbookkeeping %s: status %d, code %d: %s", e.Path, e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("ezbookkeeping %s: status %d: %s", e.Path, e.StatusCode, e.Message)
}

// envelope — спільна оболонка для успіху й помилки ezBookkeeping.
type envelope struct {
	Success      bool            `json:"success"`
	Result       json.RawMessage `json:"result"`
	ErrorCode    int             `json:"errorCode"`
	ErrorMessage string          `json:"errorMessage"`
	Path         string          `json:"path"`
}

// do відправляє запит, перевіряє відповідь і, якщо result != nil, розбирає в нього поле "result".
func (c *Client) do(ctx context.Context, method, path string, body io.Reader, result any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseUrl+apiVersion+path, body)
	if err != nil {
		return fmt.Errorf("build request %s: %w", path, err)
	}

	_, offset := time.Now().In(kyivLocation).Zone()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Timezone-Name", kyivLocation.String())
	req.Header.Set("X-Timezone-Offset", strconv.Itoa(offset/60))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request %s: %w", path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return fmt.Errorf("read response %s: %w", path, err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		// Не JSON: найчастіше HTML від reverse proxy (502, 504) або сторінка логіну.
		return &APIError{StatusCode: resp.StatusCode, Message: truncate(string(raw), 200), Path: path}
	}

	isSuccessStatus := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !isSuccessStatus || !env.Success {
		return &APIError{
			StatusCode: resp.StatusCode,
			Code:       env.ErrorCode,
			Message:    env.ErrorMessage,
			Path:       path,
		}
	}

	if result != nil {
		if err := json.Unmarshal(env.Result, result); err != nil {
			return fmt.Errorf("decode result %s: %w", path, err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
