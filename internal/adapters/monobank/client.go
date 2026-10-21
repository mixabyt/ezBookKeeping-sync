package monobank

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	base   = "https://api.monobank.ua"
	pubKey = "/bank/sync"
)

// ServerPubKey — відповідь GET /bank/sync.
// ServerKeyId — той самий ідентифікатор, що банк кладе у заголовок X-Key-Id.
type ServerPubKey struct {
	ServerKeyId    string `json:"serverKeyId"`
	ServerPubKey   string `json:"serverPubKey"`
	ServerTimeMsec int64  `json:"serverTimeMsec"`
}

// httpClient з таймаутом: цей запит стоїть у критичному шляху вебхука,
// а банк чекає відповідь 5 секунд. http.DefaultClient таймауту не має взагалі.
var httpClient = &http.Client{Timeout: 3 * time.Second}

func GetPubKey(ctx context.Context) (*ServerPubKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+pubKey, nil)
	if err != nil {
		return nil, fmt.Errorf("build request %s: %w", pubKey, err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request %s: %w", pubKey, err)
	}
	defer resp.Body.Close()

	// статус перевіряємо до розбору: інакше при 5xx у кеш ляже порожній ключ,
	// і всі наступні вебхуки відхилятимуться, поки кеш не протухне
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("monobank %s: status %d", pubKey, resp.StatusCode)
	}

	sPubKey := &ServerPubKey{}
	if err := json.NewDecoder(resp.Body).Decode(sPubKey); err != nil {
		return nil, fmt.Errorf("decode %s: %w", pubKey, err)
	}
	if sPubKey.ServerPubKey == "" || sPubKey.ServerKeyId == "" {
		return nil, fmt.Errorf("monobank %s: порожня відповідь", pubKey)
	}
	return sPubKey, nil
}
