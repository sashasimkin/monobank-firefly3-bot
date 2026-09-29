package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gitea.stuzer.link/stuzer05/go-firefly3/v2"
	"gitea.stuzer.link/stuzer05/go-monobank"
	"github.com/antihax/optional"
	"log"
	"math"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"stuzer.link/monobank-firefly3-bot/config"
	"sync"
	"time"
)

const (
	uncategorizedCategory = "Uncategorized"
	uncategorizedExpense  = "Uncategorized"
	uncategorizedIncome   = "Uncategorized income"
	refundRevenue         = "Monobank refunds"
)

var errNoMatchingTransactionRule = errors.New("no transaction rule matched Monobank statement item")
var transferWebhookWorkerOnce sync.Once
var transferWebhookQueue = make(chan monobank.WebHookResponse, 128)

// ImportTransaction imports a single webhook event. Firefly III identifies
// direction through the transaction type and expects a positive amount
// magnitude for both withdrawals and deposits.
func ImportTransaction(monobankTransaction monobank.WebHookResponse) error {
	if IsInternalTransferWebhook(monobankTransaction) {
		return importTransferWebhook(context.Background(), monobankTransaction)
	}
	_, err := importTransaction(monobankTransaction, App().Config.ImportUnmatchedTransactions)
	return err
}

// IsInternalTransferWebhook identifies events that need counterpart lookup
// before the bot can safely create a Firefly transfer.
func IsInternalTransferWebhook(monobankTransaction monobank.WebHookResponse) bool {
	rule, refund := matchTransactionRule(monobankTransaction.Data.StatementItem)
	return !refund && requiresStatementTransferPair(rule)
}

// ScheduleInternalTransferWebhook acknowledges the webhook path without
// holding Monobank's request open during its rate-limited statement lookups.
func ScheduleInternalTransferWebhook(monobankTransaction monobank.WebHookResponse) error {
	account := App().Config.GetAccountByMonobankId(monobankTransaction.Data.Account)
	if account.MonobankId == "" || account.Firefly3Name == "" {
		return errors.New("cannot find Firefly or Monobank account mapping")
	}
	transferWebhookWorkerOnce.Do(func() { go runTransferWebhookWorker() })
	select {
	case transferWebhookQueue <- monobankTransaction:
		return nil
	default:
		return errors.New("Monobank transfer webhook queue is full")
	}
}

func runTransferWebhookWorker() {
	for monobankTransaction := range transferWebhookQueue {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		err := importTransferWebhook(ctx, monobankTransaction)
		cancel()
		if err != nil {
			// The regular statement sync retries the event. Do not log payloads,
			// descriptions, amounts, account IDs, or API error bodies here.
			log.Printf("Monobank transfer webhook lookup failed; scheduled statement sync will retry")
		}
	}
}

func importTransaction(monobankTransaction monobank.WebHookResponse, allowUnmatched bool) (bool, error) {
	return importTransactionWithTransferFallback(monobankTransaction, allowUnmatched, false)
}

func importTransactionWithTransferFallback(monobankTransaction monobank.WebHookResponse, allowUnmatched, transferFallback bool) (bool, error) {
	item := monobankTransaction.Data.StatementItem
	if !isImportableStatementItem(item) {
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
	mainExternalID := externalID
	if isRefund {
		// Keep the refund-specific suffix for compatibility with any refund
		// deposits already imported by an earlier bot revision.
		mainExternalID += ":refund"
		refundAlreadyImported, err := transactionExists(mainExternalID, date)
		if err != nil {
			return false, err
		}
		// A statement poll may have imported this event as Uncategorized before
		// a matching refund rule was configured. Do not create a second record.
		alreadyImported = alreadyImported || refundAlreadyImported
	}
	if err := requireTransactionRule(rule, allowUnmatched, alreadyImported); err != nil {
		return false, err
	}
	if transferFallback && requiresStatementTransferPair(rule) {
		// A unique counterpart was not found; preserve the source event as its
		// signed inflow/outflow instead of inventing a transfer destination.
		fallbackRule := *rule
		fallbackRule.Firefly3.Type = ""
		rule = &fallbackRule
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

func isImportableStatementItem(item monobank.StatementItemsInner) bool {
	// Do not filter Monobank holds; their signed amounts use the same direction
	// handling as posted rows. Firefly stores positive magnitudes and uses the
	// transaction type to express direction.
	return item.Id != "" && (item.Amount != 0 || item.CommissionRate != 0)
}

func matchTransactionRule(item monobank.StatementItemsInner) (*config.TransactionTypes, bool) {
	return matchTransactionRuleFrom(App().Config.TransactionTypes, item)
}

func requireTransactionRule(rule *config.TransactionTypes, allowUnmatched, alreadyImported bool) error {
	if rule == nil && !allowUnmatched && !alreadyImported {
		return errNoMatchingTransactionRule
	}
	return nil
}

func matchTransactionRuleFrom(rules []config.TransactionTypes, item monobank.StatementItemsInner) (*config.TransactionTypes, bool) {
	// Refund descriptions take precedence over ordinary merchant and MCC rules
	// so a positive refund cannot be swallowed by a broad grocery category.
	if item.Amount > 0 {
		for i := range rules {
			row := &rules[i]
			if slices.Contains(row.NamesRefund, item.Description) {
				return row, true
			}
		}
	}

	for i := range rules {
		row := &rules[i]
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
			// A positive statement item matched to an expense rule is the
			// incoming side of a reversal. Keep it separate from the original
			// withdrawal instead of applying the expense rule's type to it.
			return row, item.Amount > 0 && row.Firefly3.Type == "withdrawal"
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
		if configuredDescription := strings.TrimSpace(rule.Firefly3.Description); configuredDescription != "" {
			description = configuredDescription
		}
		if rule.Firefly3.Category != "" {
			category = rule.Firefly3.Category
		}
		transactionType = rule.Firefly3.Type
		configuredSource = rule.Firefly3.Source
		configuredDestination = rule.Firefly3.Destination
	}
	description = normalizeFireflyDescription(description, "Monobank transaction")

	if refund {
		// Firefly III represents a reversal as a positive deposit. Its type
		// carries the opposite balance direction; the original withdrawal stays
		// unchanged.
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
	err := storeFireflyTransaction(transaction)
	if err == nil {
		return nil
	}
	fields := fireflyValidationFieldNamesFromError(err)
	if !slices.Equal(fields, []string{"transactions.0.description"}) {
		return formatFireflyValidationError(err)
	}

	// Some Firefly deployments may reject a source description despite local
	// normalization. Retry only this validation failure with a safe fallback;
	// never retry other validation errors or transport failures.
	transaction.Description = "Monobank transaction"
	if retryErr := storeFireflyTransaction(transaction); retryErr != nil {
		return formatFireflyValidationError(errors.Join(err, retryErr))
	}
	return nil
}

func storeFireflyTransaction(transaction firefly3.TransactionSplitStore) error {
	opts := firefly3.TransactionsApiStoreTransactionOpts{}
	_, _, err := App().Firefly3Client.TransactionsApi.StoreTransaction(context.Background(), firefly3.TransactionStore{
		ApplyRules:           true,
		ErrorIfDuplicateHash: true,
		Transactions:         []firefly3.TransactionSplitStore{transaction},
	}, &opts)
	return err
}

func formatFireflyValidationError(err error) error {
	if fields := fireflyValidationFieldNamesFromError(err); len(fields) > 0 {
		return fmt.Errorf("%w (Firefly validation fields: %s)", err, strings.Join(fields, ", "))
	}
	return err
}

func fireflyValidationFieldNamesFromError(err error) []string {
	var apiError firefly3.GenericSwaggerError
	if !errors.As(err, &apiError) {
		return nil
	}
	return fireflyValidationFieldNames(apiError.Body())
}

func fireflyValidationFieldNames(body []byte) []string {
	var response struct {
		Errors map[string]json.RawMessage `json:"errors"`
	}
	if json.Unmarshal(body, &response) != nil || len(response.Errors) == 0 {
		return nil
	}
	fields := make([]string, 0, len(response.Errors))
	for field := range response.Errors {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
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
