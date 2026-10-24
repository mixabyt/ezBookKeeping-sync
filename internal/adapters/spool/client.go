package spool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// maxResponseBody обмежує, скільки байт читаємо з однієї відповіді черги.
const maxResponseBody = 1 << 20

// Client — доступ до черги вебхуків на VPS (nginx, autoindex_format json):
//   - GET    <base>       — список файлів
//   - GET    <base><name> — один збережений запит (Request)
//   - DELETE <base><name> — 204, або 404, якщо файлу вже немає
type Client struct {
	baseURL    string // завжди закінчується на "/"
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Request — запит, який nginx зберіг, поки сервер не відповідав.
type Request struct {
	Time    string      `json:"time"`
	Method  string      `json:"method"`
	URI     string      `json:"uri"`
	Headers [][2]string `json:"headers"` // пари [ім'я, значення] в оригінальному порядку, з дублікатами
	// Body — сирі байти тіла рядком. Саме їх підписав банк: не перекодовувати перед перевіркою підпису.
	Body           string `json:"body"`
	UpstreamStatus string `json:"upstream_status"` // 502 — сервер недоступний, 504 — не відповів вчасно
}

// Header повертає заголовки як http.Header.
func (r Request) Header() http.Header {
	h := make(http.Header, len(r.Headers))
	for _, kv := range r.Headers {
		h.Add(kv[0], kv[1])
	}
	return h
}

// List повертає імена файлів у порядку надходження.
// Ім'я починається з часу в мілісекундах, тому достатньо сортування за іменем.
func (c *Client) List(ctx context.Context) ([]string, error) {
	raw, err := c.get(ctx, c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("list spool: %w", err)
	}

	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decode spool listing: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		// приховані файли nginx ще дописує
		if e.Type != "file" || strings.HasPrefix(e.Name, ".") {
			continue
		}
		names = append(names, e.Name)
	}
	sort.Strings(names)
	return names, nil
}

// Read повертає збережений запит.
func (c *Client) Read(ctx context.Context, name string) (Request, error) {
	raw, err := c.get(ctx, c.fileURL(name))
	if err != nil {
		return Request{}, fmt.Errorf("read spool file %s: %w", name, err)
	}
	var r Request
	if err := json.Unmarshal(raw, &r); err != nil {
		return Request{}, fmt.Errorf("%w: decode %s: %w", ErrDiscard, name, err)
	}
	return r, nil
}

// Delete видаляє файл з черги. Якщо його вже немає — це теж успіх.
func (c *Client) Delete(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.fileURL(name), nil)
	if err != nil {
		return fmt.Errorf("build delete %s: %w", name, err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete spool file %s: %w", name, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBody))

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("delete spool file %s: status %d", name, resp.StatusCode)
}

func (c *Client) fileURL(name string) string {
	return c.baseURL + url.PathEscape(name)
}

func (c *Client) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return raw, nil
}
