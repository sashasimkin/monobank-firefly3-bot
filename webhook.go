package main

import (
	"context"
	"encoding/json"
	"main/firefly3"
	monobank "main/monobank/api/webhook/models"
	"math"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"
)

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	var err error

	firefly3TransactionTypeWithdrawal := firefly3.WITHDRAWAL_TransactionTypeProperty
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
	if len(account.Firefly3Id) == 0 || len(account.MonobankId) == 0 {
		LogString("cannot find firefly3 or monobank ids (" + monobankTransaction.Data.Account + ")")
		w.WriteHeader(http.StatusOK)
		return
	}

	// init firefly3 client
	clientConf := firefly3.NewConfiguration()
	clientConf.BasePath = os.Getenv("FIREFLY3_API_URL")
	clientConf.AddDefaultHeader("Authorization", "Bearer "+os.Getenv("FIREFLY3_TOKEN"))
	firefly3Client := firefly3.NewAPIClient(clientConf)

	// create firefly3 transaction
	var firefly3Transactions []firefly3.TransactionSplitStore

	firefly3Transaction := firefly3.TransactionSplitStore{
		Date:     time.Unix(int64(monobankTransaction.Data.StatementItem.Time), 0).Add(time.Hour * 2),
		Notes:    string(monobankTransactionJson),
		Amount:   strconv.Itoa(int(math.Abs(math.Round(float64(monobankTransaction.Data.StatementItem.Amount/100)))) - int(math.Abs(math.Round(float64(monobankTransaction.Data.StatementItem.CommissionRate/100))))),
		SourceId: account.Firefly3Id,
	}

	// Special transaction cases
	if slices.Contains([]string{"Термінал EasyPay", "City24", "Термінал City24"}, monobankTransaction.Data.StatementItem.Description) {
		firefly3Transaction.Type_ = &firefly3TransactionTypeTransfer
		firefly3Transaction.Description = "Transfer between accounts"
		firefly3Transaction.SourceName = "Wallet cash (UAH)"   // test
		firefly3Transaction.DestinationId = account.Firefly3Id // test
		firefly3Transactions = append(firefly3Transactions, firefly3Transaction)
	} else {
		for _, row := range config.TransactionTypes {
			if slices.Contains(row.Names, monobankTransaction.Data.StatementItem.Description) || slices.Contains(row.MccCodes, monobankTransaction.Data.StatementItem.Mcc) {
				sum, _ := strconv.Atoi(firefly3Transaction.Amount)

				// check max sum
				if row.SumMax == 0 || sum <= row.SumMax {
					switch row.Firefly3.Type {
					case "withdrawal":
						firefly3Transaction.Type_ = &firefly3TransactionTypeWithdrawal
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
					firefly3Transactions = append(firefly3Transactions, firefly3Transaction)
					break
				}
			}
		}
	}

	// record transfer fee
	if monobankTransaction.Data.StatementItem.CommissionRate > 0 {
		firefly3Transactions = append(firefly3Transactions, firefly3.TransactionSplitStore{
			Type_:       &firefly3TransactionTypeWithdrawal,
			Date:        time.Unix(int64(monobankTransaction.Data.StatementItem.Time), 0).Add(time.Hour * 2),
			Notes:       string(monobankTransactionJson),
			Description: "Transfer fee",
			Amount:      strconv.Itoa(int(math.Abs(math.Round(float64(monobankTransaction.Data.StatementItem.CommissionRate / 100))))),
			SourceId:    account.Firefly3Id,
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
				LogString(err.Error())
				w.WriteHeader(http.StatusOK)
				return
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}
