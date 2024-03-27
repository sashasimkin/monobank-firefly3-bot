package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/antihax/optional"
	"github.com/joho/godotenv"
	"io"
	"log"
	"main/firefly3"
	"main/monobank/api/webhook/models"
	"math"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"
)

// https://api.monobank.ua/docs/index.html#tag/Kliyentski-personalni-dani/paths/~1personal~1statement~1{account}~1{from}~1{to}/get
// https://api-docs.firefly-iii.org/#/accounts/listAccount

// curl -X POST https://api.monobank.ua/personal/webhook -H 'Content-TransactionType: application/json' -H 'X-Token: ' -d '{"webHookUrl":"https://monobank-firefly3.stuzer.link/webhook"}'

// curl -X POST https://monobank-firefly3.stuzer.link/webhook -H 'Content-TransactionType: application/json' -d '{"test":123}'

// Configs
var ShopConfig []ShopConfigItem
var Firefy3AccountsConfig map[string]string

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	LogString("-----------------\nwebhook received!")

	// read body bytes
	body, err := io.ReadAll(r.Body)
	if err != nil {
		LogString(err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	//fmt.Println(string(body))
	//w.WriteHeader(http.StatusOK)
	//return

	//body = []byte("{\"type\":\"StatementItem\",\"data\":{\"account\":\"4723djMLsLOCzhoeYjxqRw\",\"statementItem\":{\"id\":\"5_NQ0arGAmp2pyNzvA\",\"time\":1711544958,\"description\":\"Ілля Ш.\",\"mcc\":4829,\"originalMcc\":4829,\"amount\":-572000,\"operationAmount\":-572000,\"currencyCode\":980,\"commissionRate\":22000,\"cashbackAmount\":0,\"balance\":8101246,\"hold\":true,\"receiptId\":\"EMXC-P266-90PC-EB8C\"}}}")
	LogString(string(body))

	// check empty body
	if len(string(body)) == 0 {
		LogString("empty body")
		w.WriteHeader(http.StatusOK)
		return
	}

	// parse body
	var transaction models.Transaction
	err = json.Unmarshal(body, &transaction)
	if err != nil {
		LogString(err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	//statement, err := requests.Statement(models2.StatementRequest{
	//	Account: transaction.Data.Account,
	//	From:    transaction.Data.StatementItem.Time,
	//	To:      transaction.Data.StatementItem.Time,
	//})
	//if err != nil {
	//	fmt.Printf("%+v", err.Error())
	//	w.WriteHeader(http.StatusOK)
	//	return
	//}
	//fmt.Printf("%+v", statement)

	// init firefly3 client
	clientConf := firefly3.NewConfiguration()
	clientConf.BasePath = os.Getenv("FIREFLY3_API_URL")
	clientConf.AddDefaultHeader("Authorization", "Bearer "+os.Getenv("FIREFLY3_TOKEN"))
	client := firefly3.NewAPIClient(clientConf)

	// get firefly3 account
	listOpts := firefly3.AccountsApiListAccountOpts{
		Type_: optional.NewInterface("asset"),
	}
	accounts, _, err := client.AccountsApi.ListAccount(context.Background(), &listOpts)
	if err != nil {
		LogString(err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	var account firefly3.AccountRead
	for _, row := range accounts.Data {
		if row.Attributes.Notes == transaction.Data.Account {
			account = row
		}
	}

	if len(account.Id) == 0 {
		LogString("unable to find account " + transaction.Data.Account + " in firefly3")
		w.WriteHeader(http.StatusOK)
		return
	}

	// create transaction
	var firefly3Transactions []firefly3.TransactionSplitStore

	transactionTypeWithdrawal := firefly3.WITHDRAWAL_TransactionTypeProperty
	transactionTypeTransfer := firefly3.TRANSFER_TransactionTypeProperty

	firefly3Transaction := firefly3.TransactionSplitStore{
		Type_:    &transactionTypeWithdrawal,
		Date:     time.Unix(int64(transaction.Data.StatementItem.Time), 0).Add(time.Hour * 2),
		Notes:    string(body),
		Amount:   strconv.Itoa(int(math.Abs(math.Round(float64(transaction.Data.StatementItem.Amount/100)))) - int(math.Abs(math.Round(float64(transaction.Data.StatementItem.CommissionRate/100))))),
		SourceId: account.Id,
	}

	if slices.Contains([]string{"З чорної картки"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Type_ = &transactionTypeTransfer
		firefly3Transaction.Description = "Transfer between accounts"
		firefly3Transaction.DestinationId = Firefy3AccountsConfig["Mono White"]
		firefly3Transactions = append(firefly3Transactions, firefly3Transaction)
	} else if slices.Contains([]string{"З білої картки"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Type_ = &transactionTypeTransfer
		firefly3Transaction.Description = "Transfer between accounts"
		firefly3Transaction.DestinationId = Firefy3AccountsConfig["Mono Black"]
		firefly3Transactions = append(firefly3Transactions, firefly3Transaction)
	} else if slices.Contains([]string{"Термінал City24"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Type_ = &transactionTypeTransfer
		firefly3Transaction.Description = "Transfer between accounts"
		firefly3Transaction.SourceId = Firefy3AccountsConfig["Wallet cash (UAH)"]
		firefly3Transaction.DestinationId = firefly3Transaction.SourceId
		firefly3Transactions = append(firefly3Transactions, firefly3Transaction)
	} else if slices.Contains([]string{"Банкомат DN00", "Термінал EasyPay", "City24", "Термінал City24"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Type_ = &transactionTypeTransfer
		firefly3Transaction.Description = "Transfer between accounts"
		firefly3Transaction.SourceId = Firefy3AccountsConfig["Wallet cash (UAH)"] // test
		firefly3Transaction.DestinationId = account.Id                            // test
		firefly3Transactions = append(firefly3Transactions, firefly3Transaction)
	} else {
		for _, row := range ShopConfig {
			if slices.Contains(row.Names, transaction.Data.StatementItem.Description) || slices.Contains(row.MCCCodes, transaction.Data.StatementItem.Mcc) {
				firefly3Transaction.Description = row.TransactionDescription
				firefly3Transaction.DestinationName = row.TransactionDestination
				firefly3Transaction.CategoryName = row.TransactionCategory
				firefly3Transactions = append(firefly3Transactions, firefly3Transaction)
				break
			}
		}
	}

	// record transfer fee
	if transaction.Data.StatementItem.CommissionRate > 0 {
		firefly3Transactions = append(firefly3Transactions, firefly3.TransactionSplitStore{
			Type_:       &transactionTypeWithdrawal,
			Date:        time.Unix(int64(transaction.Data.StatementItem.Time), 0).Add(time.Hour * 2),
			Notes:       string(body),
			Description: "Transfer fee",
			Amount:      strconv.Itoa(int(math.Abs(math.Round(float64(transaction.Data.StatementItem.CommissionRate / 100))))),
			SourceId:    account.Id,
		})
	}

	// log firefly3 transactions
	if len(firefly3Transactions) > 0 {
		transactionOpts := firefly3.TransactionsApiStoreTransactionOpts{}

		for _, transaction := range firefly3Transactions {
			_, _, err = client.TransactionsApi.StoreTransaction(context.Background(), firefly3.TransactionStore{
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

func main() {
	// load .env
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	// Configure
	ShopConfig, Firefy3AccountsConfig = Configure()

	// set webhook
	http.HandleFunc("/webhook", handleWebhook)

	// listen server
	fmt.Println("Webhook server listening on :3021")
	err = http.ListenAndServe(":3021", nil)
	if err != nil {
		panic(err.Error())
	}
}
