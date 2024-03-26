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

var ShopConfig []ShopConfigItem

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

	//body = []byte("{\"type\":\"StatementItem\",\"data\":{\"account\":\"4723djMLsLOCzhoeYjxqRw\",\"statementItem\":{\"id\":\"XKCZ__9Eaf0ihZ3Dwg\",\"time\":1711458930,\"description\":\"414950****7166\",\"mcc\":4829,\"originalMcc\":4829,\"amount\":-249600,\"operationAmount\":-249600,\"currencyCode\":980,\"commissionRate\":9600,\"cashbackAmount\":0,\"balance\":9223637,\"hold\":true,\"receiptId\":\"8285-K777-PBHC-T9BX\"}}}")
	LogString(string(body))

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

	// init firefly3 client
	clientConf := firefly3.NewConfiguration()
	clientConf.BasePath = os.Getenv("FIREFLY3_API_URL")
	clientConf.AddDefaultHeader("Authorization", "Bearer "+os.Getenv("FIREFLY3_TOKEN"))
	client := firefly3.NewAPIClient(clientConf)

	ctx := context.Background()

	// get firefly3 account
	listOpts := firefly3.AccountsApiListAccountOpts{
		Type_: optional.NewInterface("asset"),
	}
	accounts, _, err := client.AccountsApi.ListAccount(ctx, &listOpts)
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
		Amount:   strconv.Itoa(int(math.Abs(math.Round(float64(transaction.Data.StatementItem.Amount / 100))))),
		SourceId: account.Id,
	}

	if slices.Contains([]string{"З чорної картки"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Type_ = &transactionTypeTransfer
		firefly3Transaction.Description = "Transfer between accounts"
		firefly3Transaction.DestinationId = "60"
		firefly3Transactions = append(firefly3Transactions, firefly3Transaction)
	} else if slices.Contains([]string{"З білої картки"}, transaction.Data.StatementItem.Description) {
		firefly3Transaction.Type_ = &transactionTypeTransfer
		firefly3Transaction.Description = "Transfer between accounts"
		firefly3Transaction.DestinationId = "1"
		firefly3Transactions = append(firefly3Transactions, firefly3Transaction)
	} else {
		for _, row := range ShopConfig {
			if slices.Contains(row.Names, transaction.Data.StatementItem.Description) || slices.Contains(row.MCCCodes, transaction.Data.StatementItem.Mcc) {
				firefly3Transaction.Description = row.TransactionDescription
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

	if len(firefly3Transactions) > 0 {
		transactionOpts := firefly3.TransactionsApiStoreTransactionOpts{}
		_, _, err = client.TransactionsApi.StoreTransaction(ctx, firefly3.TransactionStore{
			ApplyRules:   true,
			Transactions: firefly3Transactions,
		}, &transactionOpts)
		if err != nil {
			LogString(err.Error())
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

	/**
	 * Bills
	 */
	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Hetzner"},
		TransactionDescription: "Hetzner: vps2",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"YouTube"},
		TransactionDescription: "YouTube membership: Latte ASMR",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Київстар +380672463500"},
		TransactionDescription: "Kyivstar: +380672463500",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Lifecell +380732463500"},
		TransactionDescription: "Lifecell: +380732463500",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"JetBrains"},
		TransactionDescription: "JetBrains: GoLand",
	})

	/**
	 * Other
	 */
	ShopConfig = append(ShopConfig, ShopConfigItem{
		MCCCodes:               []int{5411, 5499, 5451, 5422, 5412, 5921},
		Names:                  []string{"АТБ", "Велмарт", "Novus", "Glovo", "zakaz.ua", "Мегамаркет", "Сільпо"},
		TransactionDescription: "Groceries",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Аптека Доброго Дня", "Аптека оптових цін", "Аптека Копійка", "Аптека Гала", "Аптека АНЦ", "APTEKA 7", "vidshkod ekv apt12"},
		TransactionDescription: "Medications",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		MCCCodes:               []int{4131, 4111, 4112},
		Names:                  []string{"Київ Цифровий", "Київпастранс"},
		TransactionDescription: "Medications",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"McDonald’s"},
		TransactionDescription: "McDonalds",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"LeoCafe"},
		TransactionDescription: "Cafe",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Lumberjack Barberhouse"},
		TransactionDescription: "Lumberjack: haircut",
	})

	http.HandleFunc("/webhook", handleWebhook)

	fmt.Println("Webhook server listening on :3021")
	err = http.ListenAndServe(":3021", nil)
	if err != nil {
		panic(err.Error())
	}
}
