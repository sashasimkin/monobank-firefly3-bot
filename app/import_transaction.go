package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gitea.stuzer.link/stuzer05/go-firefly3/v2"
	"gitea.stuzer.link/stuzer05/go-monobank"
	"github.com/antihax/optional"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"stuzer.link/monobank-firefly3-bot/config"
	"time"
)

const (
	uncategorizedCategory = "Uncategorized"
	uncategorizedExpense  = "Uncategorized"
	uncategorizedIncome   = "Uncategorized income"
	refundRevenue         = "Monobank refunds"
)

// ImportTransaction imports a single webhook event. Firefly III identifies
// direction through the transaction type and expects a positive amount
// magnitude for both withdrawals and deposits.
func ImportTransaction(monobankTransaction monobank.WebHookResponse) error {
	_, err := importTransaction(monobankTransaction, App().Config.ImportUnmatchedTransactions)
	return err
}

func importTransaction(monobankTransaction monobank.WebHookResponse, allowUnmatched bool) (bool, error) {
	item := monobankTransaction.Data.StatementItem
	if item.Hold || (item.Amount == 0 && item.CommissionRate == 0) || item.Id == "" {
		return false, nil
	}

	accountID := monobankTransaction.Data.Account
	destAccount := App().Config.GetAccountByMonobankId(accountID)
	if destAccount.Firefly3Name == "" || destAccount.MonobankId == "" {
		return false, errors.New("cannot find Firefly or Monobank account mapping")
	}

	date := time.Unix(int64(item.Time), 0).Add(time.Hour * time.Duration(timezoneHoursDiff()))
	externalID := monobankExternalID(accountID, item.Id)
	alreadyImported, err := transactionExists(externalID, date)
	if err != nil {
		return false, err
	}

	rule, isRefund := matchTransactionRule(item)
	if rule == nil && !allowUnmatched {
		return false, nil
	}

	mainExternalID := externalID
	if isRefund {
		// A refund is its own incoming transaction. Never find, change, or delete
		// the earlier expense. Firefly III expresses incoming money as a deposit
		// with a positive amount magnitude.
		mainExternalID += ":refund"
		alreadyImported, err = transactionExists(mainExternalID, date)
		if err != nil {
			return false, err
		}
	}

	created := false
	if !alreadyImported && mainAmountMinor(item) > 0 {
		transaction := buildTransaction(item, destAccount, rule, isRefund, mainExternalID, date)
		if err := storeTransaction(transaction); err != nil {
			// A webhook and a scheduled poll can race. Treat a duplicate hash as
			// successful only after verifying the source ID exists in Firefly.
			invalidateTransactionDate(date)
			found, lookupErr := transactionExists(mainExternalID, date)
			if lookupErr != nil || !found {
				if lookupErr != nil {
					return false, errors.Join(err, lookupErr)
				}
				return false, err
			}
		} else {
			created = true
			recordImportedID(mainExternalID, date)
		}
	}

	if item.CommissionRate > 0 {
		feeID := externalID + ":commission"
		feeExists, err := transactionExists(feeID, date)
		if err != nil {
			return created, err
		}
		if !feeExists {
			fee := buildCommissionTransaction(item, destAccount, feeID, date)
			if err := storeTransaction(fee); err != nil {
				invalidateTransactionDate(date)
				found, lookupErr := transactionExists(feeID, date)
				if lookupErr != nil || !found {
					if lookupErr != nil {
						return created, errors.Join(err, lookupErr)
					}
					return created, err
				}
			} else {
				created = true
				recordImportedID(feeID, date)
			}
		}
	}

	return created, nil
}

func matchTransactionRule(item monobank.StatementItemsInner) (*config.TransactionTypes, bool) {
	for i := range App().Config.TransactionTypes {
		row := &App().Config.TransactionTypes[i]
		if slices.Contains(row.NamesRefund, item.Description) {
			return row, true
		}

		descriptionMatch := false
		if row.NamesLooseMatch {
			for _, name := range row.Names {
				if strings.HasPrefix(item.Description, name) {
					descriptionMatch = true
					break
				}
			}
		} else {
			descriptionMatch = slices.Contains(row.Names, item.Description)
		}
		if descriptionMatch || slices.Contains(row.MccCodes, int(item.Mcc)) {
			return row, false
		}
	}
	return nil, false
}

func buildTransaction(item monobank.StatementItemsInner, account config.Account, rule *config.TransactionTypes, refund bool, externalID string, date time.Time) firefly3.TransactionSplitStore {
	typeWithdrawal := firefly3.WITHDRAWAL_TransactionTypeProperty
	typeDeposit := firefly3.DEPOSIT_TransactionTypeProperty
	typeTransfer := firefly3.TRANSFER_TransactionTypeProperty

	amount := formatMinorAmount(mainAmountMinor(item))
	description := item.Description
	category := uncategorizedCategory
	transactionType := ""
	configuredSource := ""
	configuredDestination := ""
	if rule != nil {
		if rule.Firefly3.Description != "" {
			description = rule.Firefly3.Description
		}
		if rule.Firefly3.Category != "" {
			category = rule.Firefly3.Category
		}
		transactionType = rule.Firefly3.Type
		configuredSource = rule.Firefly3.Source
		configuredDestination = rule.Firefly3.Destination
	}
	if description == "" {
		description = "Monobank transaction"
	}

	if refund {
		transactionType = "deposit"
		if configuredSource == "" {
			configuredSource = refundRevenue
		}
	} else if transactionType == "" {
		if item.Amount > 0 {
			transactionType = "deposit"
		} else {
			transactionType = "withdrawal"
		}
	}

	result := firefly3.TransactionSplitStore{
		Date:         date,
		Amount:       amount,
		Description:  description,
		CategoryName: category,
		ExternalId:   externalID,
		Notes:        "Imported from Monobank statement",
	}

	switch transactionType {
	case "deposit":
		result.Type_ = &typeDeposit
		result.SourceName = configuredSource
		if result.SourceName == "" {
			result.SourceName = uncategorizedIncome
		}
		result.DestinationName = account.Firefly3Name
	case "transfer":
		result.Type_ = &typeTransfer
		result.SourceName = configuredSource
		result.DestinationName = account.Firefly3Name
		if item.Amount < 0 && result.SourceName == "" {
			result.Type_ = &typeWithdrawal
			result.DestinationName = configuredDestination
		} else if item.Amount > 0 && configuredDestination != "" {
			result.SourceName = configuredDestination
		}
	case "withdrawal":
		result.Type_ = &typeWithdrawal
		result.SourceName = account.Firefly3Name
		result.DestinationName = configuredDestination
		if result.DestinationName == "" {
			result.DestinationName = uncategorizedExpense
		}
	default:
		if item.Amount > 0 {
			result.Type_ = &typeDeposit
			result.SourceName = uncategorizedIncome
			result.DestinationName = account.Firefly3Name
		} else {
			result.Type_ = &typeWithdrawal
			result.SourceName = account.Firefly3Name
			result.DestinationName = uncategorizedExpense
		}
	}

	if item.OperationAmount != 0 && item.CurrencyCode != 0 && int(item.CurrencyCode) != currencyCodeFor(account.Currency) {
		result.ForeignAmount = formatMinorAmount(int64(math.Round(math.Abs(item.OperationAmount))))
		result.ForeignCurrencyCode = currencyCodeName(int(item.CurrencyCode))
	}
	return result
}

func buildCommissionTransaction(item monobank.StatementItemsInner, account config.Account, externalID string, date time.Time) firefly3.TransactionSplitStore {
	typeWithdrawal := firefly3.WITHDRAWAL_TransactionTypeProperty
	return firefly3.TransactionSplitStore{
		Type_:           &typeWithdrawal,
		Date:            date,
		Notes:           "Monobank transaction commission",
		Description:     "Transfer fee",
		Amount:          formatMinorAmount(int64(math.Round(math.Abs(item.CommissionRate)))),
		SourceName:      account.Firefly3Name,
		DestinationName: "Transfer fees",
		CategoryName:    uncategorizedCategory,
		ExternalId:      externalID,
	}
}

func mainAmountMinor(item monobank.StatementItemsInner) int64 {
	amount := int64(math.Round(math.Abs(item.Amount))) - int64(math.Round(math.Abs(item.CommissionRate)))
	if amount < 0 {
		return 0
	}
	return amount
}

func storeTransaction(transaction firefly3.TransactionSplitStore) error {
	opts := firefly3.TransactionsApiStoreTransactionOpts{}
	_, _, err := App().Firefly3Client.TransactionsApi.StoreTransaction(context.Background(), firefly3.TransactionStore{
		ApplyRules:           true,
		ErrorIfDuplicateHash: true,
		Transactions:         []firefly3.TransactionSplitStore{transaction},
	}, &opts)
	return err
}

func transactionExists(externalID string, date time.Time) (bool, error) {
	day := date.Format("2006-01-02")
	if _, loaded := App().loadedTransactionDates.Load(day); loaded {
		_, exists := App().processedTransactions.Load(externalID)
		return exists, nil
	}

	start := date.AddDate(0, 0, -1).Format("2006-01-02")
	end := date.AddDate(0, 0, 2).Format("2006-01-02")
	for page := int32(1); ; page++ {
		query := firefly3.TransactionsApiListTransactionOpts{
			Limit: optional.NewInt32(999),
			Page:  optional.NewInt32(page),
			Start: optional.NewString(start),
			End:   optional.NewString(end),
		}
		transactions, _, err := App().Firefly3Client.TransactionsApi.ListTransaction(context.Background(), &query)
		if err != nil {
			return false, err
		}
		for _, row := range transactions.Data {
			for _, split := range row.Attributes.Transactions {
				if split.ExternalId != "" {
					App().processedTransactions.Store(split.ExternalId, struct{}{})
				}
				if split.Notes != "" {
					var old monobank.WebHookResponse
					if json.Unmarshal([]byte(split.Notes), &old) == nil && old.Data.StatementItem.Id != "" {
						App().processedTransactions.Store(monobankExternalID(old.Data.Account, old.Data.StatementItem.Id), struct{}{})
					}
				}
			}
		}
		if transactions.Meta == nil || transactions.Meta.Pagination == nil || page >= transactions.Meta.Pagination.TotalPages {
			break
		}
	}
	App().loadedTransactionDates.Store(day, struct{}{})
	_, ok := App().processedTransactions.Load(externalID)
	return ok, nil
}

func recordImportedID(externalID string, date time.Time) {
	day := date.Format("2006-01-02")
	App().processedTransactions.Store(externalID, struct{}{})
	App().loadedTransactionDates.Store(day, struct{}{})
}

func invalidateTransactionDate(date time.Time) {
	App().loadedTransactionDates.Delete(date.Format("2006-01-02"))
}

func monobankExternalID(accountID, transactionID string) string {
	return "monobank:" + accountID + ":" + transactionID
}

func formatMinorAmount(amount int64) string {
	if amount < 0 {
		amount = -amount
	}
	return fmt.Sprintf("%d.%02d", amount/100, amount%100)
}

func timezoneHoursDiff() int {
	diff, _ := strconv.Atoi(os.Getenv("TIMEZONE_HOURS_DIFF"))
	return diff
}

func currencyCodeFor(code string) int {
	switch strings.ToUpper(code) {
	case "USD":
		return 840
	case "EUR":
		return 978
	case "UAH":
		return 980
	default:
		return 0
	}
}

func currencyCodeName(code int) string {
	switch code {
	case 840:
		return "USD"
	case 978:
		return "EUR"
	case 980:
		return "UAH"
	default:
		return ""
	}
}
