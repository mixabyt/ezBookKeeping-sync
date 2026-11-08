package domain

import "time"

// OwnerUserID — власник застосунку: йому належать дані, що були до появи кількох
// користувачів, і старий вебхук /mono-webhook без токена.
const OwnerUserID int64 = 1

// User — людина, яка користується застосунком. Кожен має власний токен ezBookkeeping,
// власну адресу вебхука, а також власні правила й транзакції.
type User struct {
	ID           int64
	Login        string
	PasswordHash string
	// MustChangePassword — вхід із тимчасовим паролем: поки людина не задасть свій, API закрите.
	MustChangePassword bool
	// SessionVersion входить у токен сесії; зміна пароля піднімає її і гасить старі сесії.
	SessionVersion int64
	// WebhookToken — випадковий секрет у шляху вебхука. Саме він, а не підпис банку,
	// визначає, чия це транзакція: Monobank підписує вебхуки всіх клієнтів одним ключем.
	WebhookToken string
	// EzbToken — порожній, поки людина не вставить його в налаштуваннях.
	EzbToken string
	// MonoWebhookRegisteredAt — нульовий час, якщо вебхук ще не реєстрували.
	MonoWebhookRegisteredAt time.Time
}
