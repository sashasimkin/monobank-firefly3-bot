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
	"net/http"
	"os"
	"strconv"
	"time"
)

// https://api.monobank.ua/docs/index.html#tag/Kliyentski-personalni-dani/paths/~1personal~1statement~1{account}~1{from}~1{to}/get
// https://api-docs.firefly-iii.org/#/accounts/listAccount

// curl -X POST https://api.monobank.ua/personal/webhook -H 'Content-Type: application/json' -H 'X-Token: ' -d '{"webHookUrl":"https://monobank-firefly3.stuzer.link/webhook"}'

// curl -X POST https://monobank-firefly3.stuzer.link/webhook -H 'Content-Type: application/json' -d '{"test":123}'

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Webhook received!")

	// read body bytes
	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusOK)
		return
	}

	//fmt.Println(string(body))
	//w.WriteHeader(http.StatusOK)
	//return

	//body := []byte("{\"type\":\"StatementItem\",\"data\":{\"account\":\"jJPAm0cfwAJv3C0I-kYpTA\",\"statementItem\":{\"id\":\"-YdIgUpWDogXEMSceQ\",\"time\":1711354414,\"description\":\"З чорної картки\",\"mcc\":4829,\"originalMcc\":4829,\"amount\":100,\"operationAmount\":100,\"currencyCode\":980,\"commissionRate\":0,\"cashbackAmount\":0,\"balance\":100,\"hold\":true}}}")

	// log body
	f, err := os.OpenFile("/tmp/monobank-filefly3.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
	}
	defer f.Close()
	if _, err := f.WriteString(string(body) + "\n"); err != nil {
		fmt.Println(err)
	}

	// parse body
	var transaction models.Transaction
	err = json.Unmarshal(body, &transaction)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusOK)
		return
	}

	// init firefly3 client
	clientConf := firefly3.NewConfiguration()
	clientConf.BasePath = "https://firefly3.stuzer.link/api"
	clientConf.AddDefaultHeader("Authorization", "Bearer "+os.Getenv("FIREFLY3_TOKEN"))
	client := firefly3.NewAPIClient(clientConf)

	ctx := context.Background()

	transactionTypeWithdrawal := firefly3.WITHDRAWAL_TransactionTypeProperty
	transactionTypeTransfer := firefly3.TRANSFER_TransactionTypeProperty
	var transactionList []firefly3.TransactionSplitStore

	// process transaction

	// get account
	listOpts := firefly3.AccountsApiListAccountOpts{
		Type_: optional.NewInterface("asset"),
	}
	accounts, _, err := client.AccountsApi.ListAccount(ctx, &listOpts)
	if err != nil {
		log.Fatalf(err.Error())
	}

	var account firefly3.AccountRead
	for _, row := range accounts.Data {
		if row.Attributes.Notes == transaction.Data.Account {
			account = row
		}
	}

	if len(account.Id) == 0 {
		fmt.Printf("unable to find account %s in firefly3\n", transaction.Data.Account)
		w.WriteHeader(http.StatusOK)
		return
	}

	// create transaction
	switch transaction.Data.StatementItem.Description {
	case "З чорної картки":
		transactionList = append(transactionList, firefly3.TransactionSplitStore{
			Type_:         &transactionTypeTransfer,
			Date:          time.Now(), // time.Unix(int64(time.Now()), 0)
			Amount:        strconv.Itoa(transaction.Data.StatementItem.Amount / 100),
			Description:   "Transfer between accounts",
			Notes:         string(body),
			SourceId:      account.Id,
			DestinationId: "60",
		})
		break
	case "З білої картки":
		transactionList = append(transactionList, firefly3.TransactionSplitStore{
			Type_:         &transactionTypeTransfer,
			Date:          time.Now(), // time.Unix(int64(time.Now()), 0)
			Amount:        strconv.Itoa(transaction.Data.StatementItem.Amount / 100),
			Description:   "Transfer between accounts",
			Notes:         string(body),
			SourceId:      account.Id,
			DestinationId: "1",
		})
		break
	case "АТБ":
	case "Велмарт":
	case "Novus":
	case "Glovo":
	case "zakaz.ua":
	case "Мегамаркет":
		transactionList = append(transactionList, firefly3.TransactionSplitStore{
			Type_:       &transactionTypeWithdrawal,
			Date:        time.Now(), // time.Unix(int64(time.Now()), 0)
			Amount:      strconv.Itoa(transaction.Data.StatementItem.Amount / 100),
			Description: "Groceries",
			Notes:       string(body),
			SourceId:    account.Id,
		})
		break
	case "Аптека Доброго Дня":
	case "Аптека оптових цін":
	case "Аптека Копійка":
	case "Аптека Гала":
	case "Аптека АНЦ":
	case "APTEKA 7":
		transactionList = append(transactionList, firefly3.TransactionSplitStore{
			Type_:       &transactionTypeWithdrawal,
			Date:        time.Now(), // time.Unix(int64(time.Now()), 0)
			Amount:      strconv.Itoa(transaction.Data.StatementItem.Amount / 100),
			Description: "Medications",
			Notes:       string(body),
			SourceId:    account.Id,
		})
		break
	case "Київ Цифровий":
	case "Київпастранс":
		transactionList = append(transactionList, firefly3.TransactionSplitStore{
			Type_:       &transactionTypeWithdrawal,
			Date:        time.Now(), // time.Unix(int64(time.Now()), 0)
			Amount:      strconv.Itoa(transaction.Data.StatementItem.Amount / 100),
			Description: "Public transport",
			Notes:       string(body),
			SourceId:    account.Id,
		})
		break
	case "McDonald's":
		transactionList = append(transactionList, firefly3.TransactionSplitStore{
			Type_:       &transactionTypeWithdrawal,
			Date:        time.Now(), // time.Unix(int64(time.Now()), 0)
			Amount:      strconv.Itoa(transaction.Data.StatementItem.Amount / 100),
			Description: "McDonalds",
			Notes:       string(body),
			SourceId:    account.Id,
		})
		break
	case "LeoCafe":
		transactionList = append(transactionList, firefly3.TransactionSplitStore{
			Type_:       &transactionTypeWithdrawal,
			Date:        time.Now(), // time.Unix(int64(time.Now()), 0)
			Amount:      strconv.Itoa(transaction.Data.StatementItem.Amount / 100),
			Description: "Cafe",
			Notes:       string(body),
			SourceId:    account.Id,
		})
		break
	case "Mafia":
		transactionList = append(transactionList, firefly3.TransactionSplitStore{
			Type_:       &transactionTypeWithdrawal,
			Date:        time.Now(), // time.Unix(int64(time.Now()), 0)
			Amount:      strconv.Itoa(transaction.Data.StatementItem.Amount / 100),
			Description: "Restaurant",
			Notes:       string(body),
			SourceId:    account.Id,
		})
		break
	case "Lumberjack Barberhouse":
		transactionList = append(transactionList, firefly3.TransactionSplitStore{
			Type_:       &transactionTypeWithdrawal,
			Date:        time.Now(), // time.Unix(int64(time.Now()), 0)
			Amount:      strconv.Itoa(transaction.Data.StatementItem.Amount / 100),
			Description: "Lumberjack: haircut",
			Notes:       string(body),
			SourceId:    account.Id,
		})
		break
	case "Hetzner":
		transactionList = append(transactionList, firefly3.TransactionSplitStore{
			Type_:       &transactionTypeWithdrawal,
			Date:        time.Now(), // time.Unix(int64(time.Now()), 0)
			Amount:      strconv.Itoa(transaction.Data.StatementItem.Amount / 100),
			Description: "Hetzner: vps2",
			Notes:       string(body),
			SourceId:    account.Id,
		})
		break
	}

	if len(transactionList) > 0 {
		transactionOpts := firefly3.TransactionsApiStoreTransactionOpts{}
		_, _, err = client.TransactionsApi.StoreTransaction(ctx, firefly3.TransactionStore{
			ApplyRules:   true,
			Transactions: transactionList,
		}, &transactionOpts)
		if err != nil {
			fmt.Println(err.Error())
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
