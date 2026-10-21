package monobank

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"ezbkTools/internal/platform/logctx"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/patrickmn/go-cache"
)

// maxBody — скільки байт тіла читаємо (захист від величезного запиту).
const maxBody = 1 << 20

// currentKeyCache — під цим ключем лежить ключ, який банк віддає зараз.
// Потрібен лише для запитів без X-Key-Id; решта кешується під самим id.
const currentKeyCache = "pubkey:current"

var (
	// ErrNoSignature — заголовка X-Sign немає взагалі.
	ErrNoSignature = errors.New("monobank: немає заголовка X-Sign")
	// ErrBadSignatureFormat — X-Sign є, але це не 64 байти base64.
	ErrBadSignatureFormat = errors.New("monobank: некоректний формат X-Sign")
	// ErrBadPubKey — /bank/sync повернув не ту точку, яку ми очікуємо.
	ErrBadPubKey = errors.New("monobank: некоректний відкритий ключ банку")
	// ErrUnknownKeyID — запит підписано ключем, якого банк уже (або ще) не віддає.
	ErrUnknownKeyID = errors.New("monobank: невідомий X-Key-Id")

	cacheStore = cache.New(5*time.Minute, 10*time.Minute)
)

// CheckSign перевіряє підпис вебхука Monobank.
//
// Банк підписує СИРІ байти тіла: ECDSA на кривій secp256k1 від SHA-256 тіла.
// X-Sign — 64 байти base64 (r‖s, по 32), X-Key-Id — яким ключем підписано
// (збігається з serverKeyId з /bank/sync). Тіло читається повністю і кладеться
// назад у r.Body, щоб хендлер міг його розпарсити.
//
// Перевіряти треба ДО парсингу JSON: будь-яка нормалізація (порядок полів,
// пробіли) змінює байти, і підпис перестає сходитись.
func CheckSign(r *http.Request) (bool, error) {
	pub, err := pubKeyFor(r.Context(), r.Header.Get("X-Key-Id"))
	if err != nil {
		logctx.From(r.Context()).Error("Помилка отримання публічного ключа", "err", err)
		return false, err
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		logctx.From(r.Context()).Error("Помилка при читання request body", "err", err)
		return false, err
	}

	r.Body = io.NopCloser(bytes.NewReader(raw))

	sign := r.Header.Get("X-Sign")
	if sign == "" {
		return false, ErrNoSignature
	}

	// підпис: base64 → рівно 64 байти, r = перші 32, s = останні 32 (не DER!)
	sig, err := base64.StdEncoding.DecodeString(sign)
	if err != nil || len(sig) != 64 {
		logctx.From(r.Context()).Warn("некоректний X-Sign",
			"err", err, "len", len(sig), "key_id", r.Header.Get("X-Key-Id"))
		return false, ErrBadSignatureFormat
	}

	var rScalar, sScalar secp256k1.ModNScalar
	if rScalar.SetByteSlice(sig[:32]) || sScalar.SetByteSlice(sig[32:]) {
		// true = значення більше за порядок кривої, такий підпис валідним бути не може
		return false, ErrBadSignatureFormat
	}

	hash := sha256.Sum256(raw)
	ok := ecdsa.NewSignature(&rScalar, &sScalar).Verify(hash[:], pub)
	if !ok {
		logctx.From(r.Context()).Warn("підпис вебхука не пройшов перевірку",
			"key_id", r.Header.Get("X-Key-Id"), "body_len", len(raw))
	}
	return ok, nil
}

// pubKeyFor віддає розібраний відкритий ключ банку для keyID із заголовка X-Key-Id.
//
// Кеш тримає вже розпарсений ключ під його власним serverKeyId, тому в банк ідемо
// лише тоді, коли підпис зроблено ключем, якого ми ще не бачили — тобто після
// ротації, а не за таймером. Запис під id не протухає: id однозначно визначає ключ.
func pubKeyFor(ctx context.Context, keyID string) (*secp256k1.PublicKey, error) {
	cacheKey := keyID
	if cacheKey == "" {
		// запит без X-Key-Id — беремо поточний ключ, але з TTL,
		// щоб після ротації не лишитись назавжди зі старим
		cacheKey = currentKeyCache
	}
	if v, found := cacheStore.Get(cacheKey); found {
		return v.(*secp256k1.PublicKey), nil
	}

	fetched, err := GetPubKey(ctx)
	if err != nil {
		return nil, err
	}

	// ключ: base64 → 65 байт нестисненої точки (0x04 ‖ X ‖ Y)
	keyRaw, err := base64.StdEncoding.DecodeString(fetched.ServerPubKey)
	if err != nil {
		return nil, errors.Join(ErrBadPubKey, err)
	}
	pub, err := secp256k1.ParsePubKey(keyRaw)
	if err != nil {
		return nil, errors.Join(ErrBadPubKey, err)
	}

	cacheStore.Set(fetched.ServerKeyId, pub, cache.NoExpiration)
	cacheStore.Set(currentKeyCache, pub, cache.DefaultExpiration)

	if keyID != "" && fetched.ServerKeyId != keyID {
		// банк уже віддає інший ключ, ніж той, яким підписано запит:
		// перевіряти цим ключем немає сенсу, підпис однаково не зійдеться
		return nil, fmt.Errorf("%w: запит підписано %s, банк віддає %s",
			ErrUnknownKeyID, keyID, fetched.ServerKeyId)
	}
	return pub, nil
}
