package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gitea.stuzer.link/stuzer05/go-firefly3/v2"
	"gitea.stuzer.link/stuzer05/go-monobank"
	"github.com/antihax/optional"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

func ImportTransaction(monobankTransaction monobank.WebHookResponse) error {
	firefly3TransactionTypeWithdrawal := firefly3.WITHDRAWAL_TransactionTypeProperty
	firefly3TransactionTypeDeposit := firefly3.DEPOSIT_TransactionTypeProperty
	firefly3TransactionTypeTransfer := firefly3.TRANSFER_TransactionTypeProperty

	timezoneHoursDiff, _ := strconv.Atoi(os.Getenv("TIMEZONE_HOURS_DIFF"))

	// get body json string (for logging)
	monobankTransactionJson, err := json.Marshal(monobankTransaction)
	if err != nil {
		return err
	}

	// find accounts
	destAccount := App().Config.GetAccountByMonobankId(monobankTransaction.Data.Account)

	// cancel if one of destAccount ids is empty
	if len(destAccount.Firefly3Name) == 0 || len(destAccount.MonobankId) == 0 {
		return errors.New("cannot find firefly3 or monobank ids (" + monobankTransaction.Data.Account + ")")
	}

	// create firefly3 transactions list
	var firefly3Transactions []firefly3.TransactionSplitStore

	// match transaction with config
	for _, row := range App().Config.TransactionTypes {

		// is refund
		if slices.Contains(row.NamesRefund, monobankTransaction.Data.StatementItem.Description) {
			opts := firefly3.TransactionsApiListTransactionOpts{
				Limit: optional.NewInt32(999),
				Type_: optional.NewInterface("withdrawal"),
				Start: optional.NewString(time.Now().AddDate(0, 0, -7).Format("2006-01-02")), // one week before
			}
			oldTransactions, _, err := App().Firefly3Client.TransactionsApi.ListTransaction(context.Background(), &opts)
			if err != nil {
				return err
			}

			// find matching transaction to delete
			isDeleted := false
			for _, tRows := range oldTransactions.Data {
				if isDeleted {
					break
				}

				for _, tRow := range tRows.Attributes.Transactions {
					// validate notes is json
					notesBytes := bytes.NewBufferString(tRow.Notes).Bytes()
					if !json.Valid(notesBytes) {
						continue
					}

					// read monobank transaction
					var monobankTransaction monobank.WebHookResponse
					err = json.Unmarshal(notesBytes, &monobankTransaction)
					if err != nil {
						continue
					}

					// find transaction
					sum := int(math.Abs(math.Round(monobankTransaction.Data.StatementItem.Amount/100))) - int(math.Abs(math.Round(monobankTransaction.Data.StatementItem.CommissionRate/100)))
					sum2, _ := strconv.ParseFloat(tRow.Amount, 64)
					if slices.Contains(row.Names, monobankTransaction.Data.StatementItem.Description) && sum == int(sum2) {
						// delete transaction
						opts := firefly3.TransactionsApiDeleteTransactionOpts{}
						_, err := App().Firefly3Client.TransactionsApi.DeleteTransaction(context.Background(), tRows.Id, &opts)
						if err != nil {
							return err
						}

						isDeleted = true
					}
				}
			}
			break
		} else {
			// check name match
			isDescriptionMatch := false
			if row.NamesLooseMatch {
				for _, name := range row.Names {
					if strings.HasPrefix(monobankTransaction.Data.StatementItem.Description, name) {
						isDescriptionMatch = true
						break
					}
				}
			} else {
				isDescriptionMatch = slices.Contains(row.Names, monobankTransaction.Data.StatementItem.Description)
			}

			// check name & mcc
			if !(isDescriptionMatch || slices.Contains(row.MccCodes, int(monobankTransaction.Data.StatementItem.Mcc))) {
				continue
			}

			// create firefly3 transaction
			firefly3Transaction := firefly3.TransactionSplitStore{
				Date:       time.Unix(int64(monobankTransaction.Data.StatementItem.Time), 0).Add(time.Hour * time.Duration(timezoneHoursDiff)),
				Notes:      string(monobankTransactionJson),
				Amount:     strconv.Itoa(int(math.Abs(math.Round(monobankTransaction.Data.StatementItem.Amount/100))) - int(math.Abs(math.Round(monobankTransaction.Data.StatementItem.CommissionRate/100)))),
				SourceName: destAccount.Firefly3Name,
			}

			// check max sum
			sum, _ := strconv.Atoi(firefly3Transaction.Amount)
			if row.SumMax > 0 && sum > row.SumMax {
				continue
			}
			// make transaction
			switch row.Firefly3.Type {
			case "withdrawal":
				firefly3Transaction.Type_ = &firefly3TransactionTypeWithdrawal
				break
			case "deposit":
				firefly3Transaction.Type_ = &firefly3TransactionTypeDeposit
				break
			case "transfer":
				firefly3Transaction.Type_ = &firefly3TransactionTypeTransfer
				break
			default:
				firefly3Transaction.Type_ = &firefly3TransactionTypeWithdrawal
			}

			firefly3Transaction.Description = row.Firefly3.Description
			firefly3Transaction.DestinationName = row.Firefly3.Destination
			firefly3Transaction.CategoryName = row.Firefly3.Category

			// swap source and destination
			if row.Firefly3.IsUseDestinationAsSource {
				firefly3Transaction.SourceName, firefly3Transaction.DestinationName = firefly3Transaction.DestinationName, firefly3Transaction.SourceName

				// when transfer between different currencies, convert
				sourceAccount := App().Config.GetAccountByFirefly3Name(firefly3Transaction.SourceName)
				if len(sourceAccount.Currency) > 0 && sourceAccount.Currency != destAccount.Currency {
					// swap amounts
					firefly3Transaction.ForeignAmount = firefly3Transaction.Amount
					firefly3Transaction.Amount = strconv.Itoa(int(math.Abs(math.Round(monobankTransaction.Data.StatementItem.OperationAmount / 100))))

					firefly3Transaction.ForeignCurrencyCode = destAccount.Currency
				}
			}

			firefly3Transactions = append(firefly3Transactions, firefly3Transaction)
			break
		}
	}
	if monobankTransaction.Data.StatementItem.CommissionRate > 0 {
		firefly3Transactions = append(firefly3Transactions, firefly3.TransactionSplitStore{
			Type_:       &firefly3TransactionTypeWithdrawal,
			Date:        time.Unix(int64(monobankTransaction.Data.StatementItem.Time), 0).Add(time.Hour * time.Duration(timezoneHoursDiff)),
			Notes:       string(monobankTransactionJson),
			Description: "Transfer fee",
			Amount:      strconv.Itoa(int(math.Abs(math.Round(monobankTransaction.Data.StatementItem.CommissionRate / 100)))),
			SourceName:  destAccount.Firefly3Name,
		})
	}

	// log firefly3 transactions
	if len(firefly3Transactions) > 0 {
		transactionOpts := firefly3.TransactionsApiStoreTransactionOpts{}

		for _, transaction := range firefly3Transactions {
			_, _, err = App().Firefly3Client.TransactionsApi.StoreTransaction(context.Background(), firefly3.TransactionStore{
				ApplyRules:   true,
				Transactions: []firefly3.TransactionSplitStore{transaction},
			}, &transactionOpts)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
