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

	//body = []byte("{\"type\":\"StatementItem\",\"data\":{\"account\":\"jJPAm0cfwAJv3C0I-kYpTA\",\"statementItem\":{\"id\":\"-YdIgUpWDogXEMSceQ\",\"time\":1711354414,\"description\":\"З чорної картки\",\"mcc\":4829,\"originalMcc\":4829,\"amount\":100,\"operationAmount\":100,\"currencyCode\":980,\"commissionRate\":0,\"cashbackAmount\":0,\"balance\":100,\"hold\":true}}}")
	logString(string(body))

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
	var transactionList []firefly3.TransactionSplitStore

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

	t := firefly3.TransactionSplitStore{
		Type_:    &transactionTypeWithdrawal,
		Date:     time.Now(), // time.Unix(int64(time.Now()), 0)
		Notes:    string(body),
		Amount:   strconv.Itoa(int(math.Round(float64(transaction.Data.StatementItem.Amount / 100)))),
		SourceId: account.Id,
	}

	// create transaction
	switch transaction.Data.StatementItem.Description {
	case "З чорної картки":
		t.Type_ = &transactionTypeTransfer
		t.Description = "Transfer between accounts"
		t.DestinationId = "60"
		break
	case "З білої картки":
		t.Type_ = &transactionTypeTransfer
		t.Description = "Transfer between accounts"
		t.DestinationId = "1"
		break
	case "АТБ":
	case "Велмарт":
	case "Novus":
	case "Glovo":
	case "zakaz.ua":
	case "Мегамаркет":
		t.Description = "Groceries"
		break
	case "Аптека Доброго Дня":
	case "Аптека оптових цін":
	case "Аптека Копійка":
	case "Аптека Гала":
	case "Аптека АНЦ":
	case "APTEKA 7":
		t.Description = "Medications"
		break
	case "Київ Цифровий":
	case "Київпастранс":
		t.Description = "Public transport"
		break
	case "McDonald's":
		t.Description = "McDonalds"
		break
	case "LeoCafe":
		t.Description = "Cafe"
		break
	case "Mafia":
		t.Description = "Restaurant"
		break
	case "Lumberjack Barberhouse":
		t.Description = "Lumberjack: haircut"
		break
	case "Hetzner":
		t.Description = "Hetzner: vps2"
	case "YouTube":
		t.Description = "YouTube membership: Latte ASMR"
	case "Київстар +380672463500":
		t.Description = "Kyivstar: +380672463500"
	case "Lifecell +380732463500":
		t.Description = "Lifecell: +380732463500"
		break
	}

	if len(t.Description) > 0 {
		transactionList = append(transactionList, t)

		transactionOpts := firefly3.TransactionsApiStoreTransactionOpts{}
		_, _, err = client.TransactionsApi.StoreTransaction(ctx, firefly3.TransactionStore{
			ApplyRules:   true,
			Transactions: transactionList,
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
