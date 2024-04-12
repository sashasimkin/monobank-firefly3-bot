package main

import (
	"context"
	"flag"
	"fmt"
	"gitea.stuzer.link/stuzer05/go-firefly3/v2"
	"gitea.stuzer.link/stuzer05/go-monobank"
	"github.com/antihax/optional"
	"github.com/joho/godotenv"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
)

// https://api.monobank.ua/docs/index.html#tag/Kliyentski-personalni-dani/paths/~1personal~1statement~1{account}~1{from}~1{to}/get
// https://api-docs.firefly-iii.org/#/accounts/listAccount

// curl -X POST https://api.monobank.ua/personal/webhook -H 'Content-Type: application/json' -H 'X-Token: ' -d '{"webHookUrl":"https://monobank-firefly3.stuzer.link/webhook"}'

// curl -X POST https://monobank-firefly3.io.stuzer.link/webhook -H 'Content-Type: application/json' -d '{"test":123}'

func main() {
	// load .env
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("error loading .env file")
	}

	// test config read
	_, err = ReadConfig(os.Getenv("CONFIG_PATH"))
	if err != nil {
		log.Fatalf("cannot read config - " + err.Error())
	}

	// flags
	flagMonobankDoTransaction := flag.String("monobank-transaction", "", "run monobank transaction JSON manually")
	flagMonobankListAccounts := flag.Bool("monobank-list-accounts", false, "list monobank accounts")
	flagFirefly3ListAccounts := flag.Bool("firefly3-list-accounts", false, "list firefly3 accounts")

	flag.Parse()

	// init monobank client
	monobankClientConf := monobank.NewConfiguration()
	monobankClient := monobank.NewAPIClient(monobankClientConf)

	// init firefly3 client
	clientConf := firefly3.NewConfiguration()
	clientConf.BasePath = os.Getenv("FIREFLY3_API_URL")
	clientConf.AddDefaultHeader("Authorization", "Bearer "+os.Getenv("FIREFLY3_TOKEN"))
	firefly3Client := firefly3.NewAPIClient(clientConf)

	// manual transaction
	if *flagMonobankListAccounts {
		// get monobank accounts
		req := monobank.ApiPersonalClientInfoGetRequest{}
		req = req.XToken(os.Getenv("MONOBANK_TOKEN"))
		res, _, err := monobankClient.DefaultApi.PersonalClientInfoGetExecute(req)
		if err != nil {
			log.Fatalln(err.Error())
		}

		// list accounts
		for _, row := range res.Accounts {
			fmt.Printf("%v\t%v\n", *row.Id, (*row.MaskedPan)[0])
		}
	} else if *flagFirefly3ListAccounts {
		// get firefly3 accounts
		req := firefly3.AccountsApiListAccountOpts{
			Limit: optional.NewInt32(9999),
		}
		res, _, err := firefly3Client.AccountsApi.ListAccount(context.Background(), &req)
		if err != nil {
			log.Fatalln(err.Error())
		}

		// list accounts
		for _, row := range res.Data {
			if row.Attributes.Active && (*row.Attributes.Type_ == firefly3.ASSET_ShortAccountTypeProperty) {
				fmt.Printf("%v\t%v\n", row.Id, row.Attributes.Name)
			}
		}
	} else if len(*flagMonobankDoTransaction) > 0 {
		w := httptest.NewRecorder()

		r := &http.Request{
			Method: http.MethodPost,
			Header: make(http.Header),
			Body:   io.NopCloser(strings.NewReader(*flagMonobankDoTransaction)),
		}
		r.Header.Set("Content-Type", "application/json")

		handleWebhook(w, r)

		// @todo error logging
		//response := w.Result()
		//if response.StatusCode != http.StatusOK {
		//	os.Exit(1)
		//}
	} else {
		webhookLocalUrl := fmt.Sprintf("/webhook/%s", os.Getenv("MONOBANK_WEBHOOK_SECRET"))
		webhookUrl := fmt.Sprintf("https://%s/webhook/%s", os.Getenv("MONOBANK_WEBHOOK_DOMAIN"), os.Getenv("MONOBANK_WEBHOOK_SECRET"))

		// register monobank webhook
		req := monobank.ApiPersonalWebhookPostRequest{}
		req = req.XToken(os.Getenv("MONOBANK_TOKEN"))
		req = req.SetWebHook(monobank.SetWebHook{WebHookUrl: &webhookUrl})
		_, err := monobankClient.DefaultApi.PersonalWebhookPostExecute(req)
		if err != nil {
			log.Fatalln("failed to register monobank webhook")
		}

		// set webhook
		http.HandleFunc(webhookLocalUrl, handleWebhook)

		// listen server
		fmt.Println("webhook server listening on " + os.Getenv("LISTEN"))
		fmt.Println("webhook url " + webhookUrl)
		err = http.ListenAndServe(os.Getenv("LISTEN"), nil)
		if err != nil {
			log.Fatalln(err.Error())
		}
	}
}
