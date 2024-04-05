package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/antihax/optional"
	"math"
	"net/http"
	"os"
	"slices"
	"strconv"
	"stuzer.link/monobank-firefly3-bot/firefly3"
	monobank "stuzer.link/monobank-firefly3-bot/monobank/api/webhook/models"
	"time"
)

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	var err error

	firefly3TransactionTypeWithdrawal := firefly3.WITHDRAWAL_TransactionTypeProperty
	firefly3TransactionTypeDeposit := firefly3.DEPOSIT_TransactionTypeProperty
	firefly3TransactionTypeTransfer := firefly3.TRANSFER_TransactionTypeProperty

	// read request
	var monobankTransaction monobank.Transaction
	monobankTransaction, err = readResponseBody(r)
	if err != nil {
		LogString(err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	// get body json string (for logging)
	monobankTransactionJson, err := json.Marshal(monobankTransaction)
	if err != nil {
		LogString(err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	// read config
	var config Config
	config, err = ReadConfig(os.Getenv("CONFIG_PATH"))
	if err != nil {
		LogString(err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	// find accounts
	account := ConfigGetAccountByMonobankId(config, monobankTransaction.Data.Account)

	// cancel if one of account ids is empty
	if len(account.Firefly3Name) == 0 || len(account.MonobankId) == 0 {
		LogString("cannot find firefly3 or monobank ids (" + monobankTransaction.Data.Account + ")")
		w.WriteHeader(http.StatusOK)
		return
	}

	// init firefly3 client
	clientConf := firefly3.NewConfiguration()
	clientConf.BasePath = os.Getenv("FIREFLY3_API_URL")
	clientConf.AddDefaultHeader("Authorization", "Bearer "+os.Getenv("FIREFLY3_TOKEN"))
	firefly3Client := firefly3.NewAPIClient(clientConf)

	// create firefly3 transactions list
	var firefly3Transactions []firefly3.TransactionSplitStore

	// match transaction with config
	for _, row := range config.TransactionTypes {

		// is refund
		if slices.Contains(row.NamesRefund, monobankTransaction.Data.StatementItem.Description) {
			opts := firefly3.TransactionsApiListTransactionOpts{
				Limit: optional.NewInt32(999),
				Type_: optional.NewInterface("withdrawal"),
				Start: optional.NewString(time.Now().AddDate(0, 0, -7).Format("2006-01-02")), // one week before
			}
			oldTransactions, _, err := firefly3Client.TransactionsApi.ListTransaction(context.Background(), &opts)
			if err != nil {
				LogString(err.Error())
				w.WriteHeader(http.StatusOK)
				return
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
					var monobankTransaction monobank.Transaction
					err = json.Unmarshal(notesBytes, &monobankTransaction)
					if err != nil {
						continue
					}

					// find transaction
					sum := int(math.Abs(math.Round(float64(monobankTransaction.Data.StatementItem.Amount/100)))) - int(math.Abs(math.Round(float64(monobankTransaction.Data.StatementItem.CommissionRate/100))))
					sum2, _ := strconv.ParseFloat(tRow.Amount, 64)
					if slices.Contains(row.Names, monobankTransaction.Data.StatementItem.Description) && sum == int(sum2) {
						// delete transaction
						opts := firefly3.TransactionsApiDeleteTransactionOpts{}
						firefly3Client.TransactionsApi.DeleteTransaction(context.Background(), tRows.Id, &opts)

						isDeleted = true
					}
				}
			}
			break
		} else {
			// check name & mcc
			if !(slices.Contains(row.Names, monobankTransaction.Data.StatementItem.Description) || slices.Contains(row.MccCodes, monobankTransaction.Data.StatementItem.Mcc)) {
				continue
			}

			// create firefly3 transaction
			firefly3Transaction := firefly3.TransactionSplitStore{
				Date:       time.Unix(int64(monobankTransaction.Data.StatementItem.Time), 0).Add(time.Hour * 2),
				Notes:      string(monobankTransactionJson),
				Amount:     strconv.Itoa(int(math.Abs(math.Round(float64(monobankTransaction.Data.StatementItem.Amount/100)))) - int(math.Abs(math.Round(float64(monobankTransaction.Data.StatementItem.CommissionRate/100))))),
				SourceName: account.Firefly3Name,
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
			}

			firefly3Transactions = append(firefly3Transactions, firefly3Transaction)
			break
		}
	}
	if monobankTransaction.Data.StatementItem.CommissionRate > 0 {
		firefly3Transactions = append(firefly3Transactions, firefly3.TransactionSplitStore{
			Type_:       &firefly3TransactionTypeWithdrawal,
			Date:        time.Unix(int64(monobankTransaction.Data.StatementItem.Time), 0).Add(time.Hour * 2),
			Notes:       string(monobankTransactionJson),
			Description: "Transfer fee",
			Amount:      strconv.Itoa(int(math.Abs(math.Round(float64(monobankTransaction.Data.StatementItem.CommissionRate / 100))))),
			SourceName:  account.Firefly3Name,
		})
	}

	// log firefly3 transactions
	if len(firefly3Transactions) > 0 {
		transactionOpts := firefly3.TransactionsApiStoreTransactionOpts{}

		for _, transaction := range firefly3Transactions {
			_, _, err = firefly3Client.TransactionsApi.StoreTransaction(context.Background(), firefly3.TransactionStore{
				ApplyRules:   true,
				Transactions: []firefly3.TransactionSplitStore{transaction},
			}, &transactionOpts)
			if err != nil {
				fmt.Printf(err.Error())
				w.WriteHeader(http.StatusOK)
				return
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}
