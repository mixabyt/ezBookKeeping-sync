# Архітектура ezbk-tools

Мікросервіс, який приймає транзакції з Monobank (вебхук) і записує їх в ezBookkeeping.

Цей документ пояснює **чому** код розкладений саме так, а не лише **де** що лежить.
Коли не знаєш, куди покласти новий файл — відповідь має бути тут.

---

## 1. Потік даних

```
                 ┌──────────────── ezbk-tools ─────────────────┐
                 │                                             │
 Monobank ─POST─►│ httpapi ──► app.Enqueue ──► sqlite (jobs)   │
                 │                                  │          │
                 │                              worker         │
                 │                                  ▼          │
 ezBookkeeping ◄─┼─ ezbookkeeping ◄── app.Process ◄─┘          │
                 │                       │                     │
                 │                       └─► sqlite (categories)│
                 └─────────────────────────────────────────────┘
```

1. Monobank шле `POST /mono-webhook` з транзакцією.
2. `httpapi` парсить JSON у `StatementItemDTO`, перетворює на `domain.BankTransaction`
   і викликає `app.Enqueue`.
3. `Enqueue` зберігає джобу в SQLite зі статусом `pending`. Хендлер одразу відповідає `200`.
4. `worker` у фоні бере `pending` джоби і для кожної викликає `app.Process`.
5. `Process` знаходить категорію за MCC (таблиця в БД), відправляє транзакцію
   в ezBookkeeping і ставить джобі `done` або `failed`.

**Чому не відправляти в ezBookkeeping прямо з хендлера?**
- Monobank чекає швидку відповідь. Якщо ezBookkeeping повільний чи лежить — Monobank
  вирішить, що вебхук не дійшов, і почне повторювати (а при багатьох помилках може відключити вебхук).
- Якщо сервіс впаде посеред запиту — транзакція загубиться. З джобою в БД вона чекатиме рестарту.

Цей підхід називається **persistent queue / transactional outbox**: спочатку надійно
записати намір, потім виконати його асинхронно.

---

## 2. Шари і правило залежностей

```
adapters/httpapi ───┐
adapters/sqlite ────┼──►  app  ──►  domain
adapters/ezbookkeeping ┘
cmd (main) ──► усе
```

**Головне правило: залежності йдуть тільки всередину (зліва направо).**
`domain` не знає ні про що. `app` знає тільки про `domain`. Адаптери знають про `app`/`domain`,
але **не знають одне про одного**.

| Пакет | Відповідальність | Може імпортувати |
|---|---|---|
| `internal/domain` | Типи бізнес-світу: `BankTransaction`, `Job`, `JobStatus`. Без json/sql тегів, без gin. | нічого |
| `internal/app` | Бізнес-логіка (use cases): `SyncService`, `worker`. Оголошує інтерфейси, які йому потрібні (`ports.go`). | `domain` |
| `internal/adapters/httpapi` | Вхід ззовні: gin-сервер, хендлери, DTO Monobank, мапінг DTO → domain. | `app`, `domain` |
| `internal/adapters/sqlite` | Реалізація `JobRepository`, `CategoryRepository`. | `app`, `domain` |
| `internal/adapters/ezbookkeeping` | HTTP-клієнт ezBookkeeping, його DTO, мапінг domain → request. Реалізує `Bookkeeper`. | `domain` |
| `internal/config` | Читання env / `.env`. | нічого з `internal` |
| `cmd/ezbk-tools` | **Composition root**: створює всі об'єкти, з'єднує їх, запускає, гасить. | усе |

Ця архітектура відома як **Ports & Adapters** (Hexagonal) / **Clean Architecture**:
- **Port** — інтерфейс, який оголошує ядро (`app.JobRepository`, `app.Bookkeeper`).
- **Adapter** — конкретна реалізація порту для зовнішнього світу (`sqlite`, `ezbookkeeping`, `httpapi`).

### Як вирішити, куди покласти файл

1. Це про формат/протокол зовнішньої системи (JSON Monobank, SQL, HTTP ezBookkeeping)? → `adapters/<система>`.
2. Це правило «що робити з транзакцією» (яку категорію взяти, коли ретраїти, що таке дубль)? → `app`.
3. Це просто опис даних, які розуміє весь сервіс? → `domain`.
4. Це «створи X і передай у Y»? → `cmd/ezbk-tools/main.go`.

Якщо файл хоче імпортувати щось проти стрілки — це сигнал, що він лежить не там
або що потрібен новий інтерфейс у `app`.

---

## 3. Інтерфейси оголошує споживач (Go-ідіома)

У Java/C# інтерфейс зазвичай лежить біля реалізації (`store.Store` + `repository.store`).
У Go навпаки: **інтерфейс оголошує той, хто ним користується**.

```go
// internal/app/ports.go — app каже: «мені потрібне щось, що вміє ось це»
type Bookkeeper interface {
    AddTransaction(ctx context.Context, tx domain.BankTransaction, categoryID string) error
}
```

`ezbookkeeping.Client` нічого не знає про `app.Bookkeeper` — він просто має метод
з такою сигнатурою, і Go автоматично вважає, що він інтерфейс реалізує.

Що це дає:
- `app` можна тестувати з фейковим `Bookkeeper` без мережі.
- Можна замінити SQLite на Postgres, не торкаючись `app`.
- Інтерфейси маленькі — рівно те, що потрібно, а не «все, що вміє клієнт».

---

## 4. Мапінг між шарами

Кожна межа має свій тип даних:

```
StatementItemDTO   ──(httpapi)──►   domain.BankTransaction   ──(ezbookkeeping)──►   TransactionCreateRequest
   JSON Monobank                         наша модель                                  JSON ezBookkeeping
```

Мапінг живе **в адаптері**, бо тільки адаптер знає зовнішній формат.
Якщо Monobank змінить поле — правиться лише `httpapi`. `app` і `ezbookkeeping` не помітять.

---

## 5. Worker і черга

- Джерело правди — **таблиця `jobs`**, а не канал у пам'яті.
- Воркер:
  - або періодично опитує БД (`FetchPending` раз на N секунд) — найпростіше;
  - або спить, поки `Enqueue` не «штурхне» його через `chan struct{}` з буфером 1
    (плюс опитування як страховка після рестарту).
- Після обробки: `MarkDone` або `MarkFailed` (+ `Attempts++`, `LastError`).
- Ретраї мережевих помилок — у клієнті ezBookkeeping (exponential backoff + jitter).
  Ретраї «на рівні джоби» (через хвилину/годину) — у воркері за `Attempts`.

---

## 6. Ідемпотентність (захист від дублів)

Monobank може надіслати той самий вебхук кілька разів (наприклад, якщо не отримав 200 вчасно).

Варіанти:
- **UNIQUE на ID транзакції Monobank у `jobs`** + джоби не видаляються.
  Повторний вебхук просто не вставиться (`INSERT ... ON CONFLICT DO NOTHING`). Найпростіше.
- **Окрема idempotency table** — коли ключ ідемпотентності не збігається з сутністю
  або треба зберігати відповідь на повторний запит.

Важливо: навіть з UNIQUE в `jobs` дубль можливий на іншому кінці — якщо ezBookkeeping
записав транзакцію, а сервіс впав до `MarkDone`. Тоді воркер відправить її ще раз.
Це класична проблема **at-least-once delivery**; варто подумати, як її закрити
(наприклад, перевіряти в ezBookkeeping за коментарем/тегом з ID Monobank).

---

## 7. Життєвий цикл (cmd/ezbk-tools/main.go)

```
config → db → repos → ezbookkeeping client → SyncService → worker → http server
                                                                         │
SIGINT/SIGTERM ──► зупинити http (Shutdown) ──► зупинити worker (ctx cancel, дочекатися) ──► закрити db
```

Порядок зупинки — зворотний до запуску: спочатку перестати приймати нове, потім доробити
поточне, потім закрити ресурси.

---

## 8. Що тут можна спробувати нового

Ідеї задач, які ця архітектура дозволяє робити ізольовано, по одному шматку:

- [ ] **Unit-тести `app`** з фейковими репозиторіями (table-driven tests).
- [ ] **Інтеграційні тести `sqlite`** на справжній БД у тимчасовому файлі.
- [ ] **Міграції** через goose / golang-migrate замість ручного SQL.
- [ ] **Graceful shutdown воркера** через `context` + `sync.WaitGroup` / `errgroup`.
- [ ] **Exponential backoff з jitter** і розрізнення помилок: які ретраїти (5xx, timeout), які ні (4xx).
- [ ] **Пул воркерів** (N горутин) і що робити, щоб дві горутини не взяли одну джобу.
- [ ] **Захист вебхука**: секретний шлях/токен в URL, перевірка, що `account` — твій рахунок.
- [ ] **Структуроване логування** через `log/slog`.
- [ ] **Метрики** (Prometheus): скільки джоб pending/failed, час обробки.
- [ ] **Ще один банк** як новий адаптер — найкращий тест, чи архітектура справді працює:
      якщо довелося змінювати `app` — щось протекло.
- [ ] **Docker / docker-compose** разом з ezBookkeeping.
