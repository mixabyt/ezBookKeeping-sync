package ezbookkeeping

import (
	"bytes"
	"context"
	"encoding/json"
	"ezbkTools/internal/domain"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"time"
)

type Client struct {
	baseUrl    string
	httpClient *http.Client
	token      string
}

var (
	baseDelay   = time.Second * 5
	maxAttempts = 5
	maxDelay    = 10 * time.Second

	apiVersion = "/api/v1"

	// Tokens
	getTokens   = "/tokens/list.json"
	revokeToken = "/tokens/revoke.json"

	// Accounts
	getAccounts = "/accounts/list.json"
	addAccount  = "/accounts/add.json"

	// Transaction categories
	getTransactionCategories = "/transaction/categories/list.json"
	addTransactionCategory   = "/transaction/categories/add.json"

	// Transaction tags
	getTransactionTags = "/transaction/tags/list.json"
	addTransactionTag  = "/transaction/tags/add.json"

	// Transactions
	getTransactions    = "/transactions/list.json"
	getAllTransactions = "/transactions/list/all.json"
	addNewTransaction  = "/transactions/add.json"
	modifyTransaction  = "/transactions/modify.json"
	deleteTransaction  = "/transactions/delete.json"

	// Exchange rates
	getLatestExchangeRates       = "/exchange_rates/latest.json"
	updateUserCustomExchangeRate = "/exchange_rates/user_custom/update.json"
	deleteUserCustomExchangeRate = "/exchange_rates/user_custom/delete.json"
)

func NewEzBookkeepingClient(token, baseUrl string) *Client {
	return &Client{
		baseUrl: baseUrl,
		token:   token,
		httpClient: &http.Client{
			Timeout: baseDelay,
		},
	}
}

func (c *Client) AddTransaction(ctx context.Context, tx *domain.BankTransaction, categoryID string) error {
	txDto := c.FromBankTx(tx)
	txDto.CategoryID = categoryID
	tx.AccountID = defaultSourceAccountID

	body, err := json.Marshal(txDto)
	if err != nil {
		return fmt.Errorf("marshal transaction %s: %w", tx.ID, err)
	}

	if err := c.do(ctx, http.MethodPost, addNewTransaction, bytes.NewReader(body), nil); err != nil {
		return fmt.Errorf("add transaction %s: %w", tx.ID, err)
	}
	return nil
}

func (c *Client) backoffRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err := c.httpClient.Do(req)
		if err == nil {
			log.Println("Успіх!")
			return resp, nil
		}

		log.Printf("Спроба %d невдала: %v\n", attempt+1, err)

		if attempt == maxAttempts-1 {
			fmt.Println("Вичерпано ліміт спроб.")
			break
		}

		backoffDuration := float64(baseDelay) * math.Pow(2, float64(attempt))
		if backoffDuration > float64(maxDelay) {
			backoffDuration = float64(maxDelay)
		}

		jitterValue := rand.Int63n(int64(backoffDuration))
		sleepTime := time.Duration(jitterValue)

		fmt.Printf("Очікування %v перед наступною спробою...\n", sleepTime)

		select {
		case <-time.After(sleepTime):
		case <-ctx.Done():
			fmt.Println("Операцію скасовано через контекст")
			return nil, ctx.Err()
		}
	}
	return nil, nil
}
