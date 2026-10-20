package ezbookkeeping

import (
	"ezbkTools/internal/adapters/ezbookkeeping/dto"
	"ezbkTools/internal/domain"
	"time"
	_ "time/tzdata" // база часових поясів у бінарнику (на Windows її може не бути)
)

// TODO: брати з мапінгу MCC → категорія та рахунок Monobank → рахунок ezBookkeeping
const (
	defaultSourceAccountID = "3848785273705988096"
)

var kyivLocation = mustLoadLocation("Europe/Kyiv")

func (c *Client) FromBankTx(transaction *domain.BankTransaction) *dto.TransactionCreateRequest {
	txType := dto.TransactionTypeIncome
	amount := transaction.Amount
	if amount < 0 {
		txType = dto.TransactionTypeExpense
		amount = -amount
	}

	now := time.Now().In(kyivLocation)
	_, offsetSeconds := now.Zone()

	return &dto.TransactionCreateRequest{
		Type: txType,
		//CategoryID:      categoryId,
		Time:      now.Unix(),
		UtcOffset: offsetSeconds / 60,
		//SourceAccountId: defaultSourceAccountID,
		SourceAmount: int(amount),
		Comment:      &transaction.Description,
	}
}

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}
