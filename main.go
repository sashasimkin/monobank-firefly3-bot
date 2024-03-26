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

// curl -X POST https://api.monobank.ua/personal/webhook -H 'Content-Type: application/json' -H 'X-Token: ' -d '{"webHookUrl":"https://monobank-firefly3.stuzer.link/webhook"}'

// curl -X POST https://monobank-firefly3.stuzer.link/webhook -H 'Content-Type: application/json' -d '{"test":123}'

func logString(str string) {
	if len(os.Getenv("LOG_FILE")) == 0 {
		return
	}

	f, err := os.OpenFile(os.Getenv("LOG_FILE"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
	}
	defer f.Close()

	if _, err := f.WriteString(str + "\n"); err != nil {
		fmt.Println(err)
	}
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	logString("-----------------\nwebhook received!")

	// read body bytes
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logString(err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	//fmt.Println(string(body))
	//w.WriteHeader(http.StatusOK)
	//return

	//body = []byte("{\"type\":\"StatementItem\",\"data\":{\"account\":\"4723djMLsLOCzhoeYjxqRw\",\"statementItem\":{\"id\":\"AQj_9aV8GxUS9opPJQ\",\"time\":1711448198,\"description\":\"Київ Цифровий\",\"mcc\":4111,\"originalMcc\":4111,\"amount\":-800,\"operationAmount\":-800,\"currencyCode\":980,\"commissionRate\":0,\"cashbackAmount\":0,\"balance\":9528749,\"hold\":true,\"receiptId\":\"ABBT-6020-K1H7-A2ME\"}}}")
	logString(string(body))

	if len(string(body)) == 0 {
		logString("empty body")
		w.WriteHeader(http.StatusOK)
		return
	}

	// parse body
	var transaction models.Transaction
	err = json.Unmarshal(body, &transaction)
	if err != nil {
		logString(err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	// init firefly3 client
	clientConf := firefly3.NewConfiguration()
	clientConf.BasePath = os.Getenv("FIREFLY3_API_URL")
	clientConf.AddDefaultHeader("Authorization", "Bearer "+os.Getenv("FIREFLY3_TOKEN"))
	client := firefly3.NewAPIClient(clientConf)

	ctx := context.Background()

	// process transaction
	transactionTypeWithdrawal := firefly3.WITHDRAWAL_TransactionTypeProperty
	transactionTypeTransfer := firefly3.TRANSFER_TransactionTypeProperty

	// get firefly3 account
	listOpts := firefly3.AccountsApiListAccountOpts{
		Type_: optional.NewInterface("asset"),
	}
	accounts, _, err := client.AccountsApi.ListAccount(ctx, &listOpts)
	if err != nil {
		logString(err.Error())
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
		logString("unable to find account " + transaction.Data.Account + " in firefly3")
		w.WriteHeader(http.StatusOK)
		return
	}

	// create transaction
	firefly3Transaction := firefly3.TransactionSplitStore{
		Type_:    &transactionTypeWithdrawal,
		Date:     time.Unix(int64(transaction.Data.StatementItem.Time), 0).Add(time.Hour * 2),
		Notes:    string(body),
		Amount:   strconv.Itoa(int(math.Abs(math.Round(float64(transaction.Data.StatementItem.Amount / 100))))),
		SourceId: account.Id,
	}

	if slices.Contains([]string{"З чорної картки"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Type_ = &transactionTypeTransfer
		firefly3Transaction.Description = "Transfer between accounts"
		firefly3Transaction.DestinationId = "60"
	} else if slices.Contains([]string{"З білої картки"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Type_ = &transactionTypeTransfer
		firefly3Transaction.Description = "Transfer between accounts"
		firefly3Transaction.DestinationId = "1"
	} else if slices.Contains([]string{"АТБ", "Велмарт", "Novus", "Glovo", "zakaz.ua", "Мегамаркет"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Description = "Groceries"
	} else if slices.Contains([]string{"Аптека Доброго Дня", "Аптека оптових цін", "Аптека Копійка", "Аптека Гала", "Аптека АНЦ", "APTEKA 7"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Description = "Medications"
	} else if slices.Contains([]string{"Київ Цифровий", "Київпастранс"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Description = "Public transport"
	} else if slices.Contains([]string{"McDonald's"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Description = "McDonalds"
	} else if slices.Contains([]string{"LeoCafe"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Description = "Cafe"
	} else if slices.Contains([]string{"Lumberjack Barberhouse"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Description = "Lumberjack: haircut"
	} else if slices.Contains([]string{"Hetzner"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Description = "Hetzner: vps2"
	} else if slices.Contains([]string{"YouTube"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Description = "YouTube membership: Latte ASMR"
	} else if slices.Contains([]string{"Київстар +380672463500"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Description = "Kyivstar: +380672463500"
	} else if slices.Contains([]string{"Lifecell +380732463500"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Description = "Lifecell: +380732463500"
	}

	if len(firefly3Transaction.Description) > 0 {
		transactionOpts := firefly3.TransactionsApiStoreTransactionOpts{}
		_, _, err = client.TransactionsApi.StoreTransaction(ctx, firefly3.TransactionStore{
			ApplyRules:   true,
			Transactions: []firefly3.TransactionSplitStore{firefly3Transaction},
		}, &transactionOpts)
		if err != nil {
			logString(err.Error())
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	http.HandleFunc("/webhook", handleWebhook)
	fmt.Println("Webhook server listening on :3021")
	http.ListenAndServe(":3021", nil)
}
