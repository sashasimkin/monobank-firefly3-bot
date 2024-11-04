package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"gitea.stuzer.link/stuzer05/go-firefly3/v2"
	"gitea.stuzer.link/stuzer05/go-monobank"
	"github.com/antihax/optional"
	"github.com/joho/godotenv"
	"log"
	"net/http"
	"os"
	"stuzer.link/monobank-firefly3-bot/app"
)

func main() {
	// load .env
	godotenv.Load(".env")

	// init app
	app.Init()

	// flags
	flagMonobankDoTransaction := flag.String("monobank-transaction", "", "run monobank transaction JSON manually")
	flagMonobankListAccounts := flag.Bool("monobank-list-accounts", false, "list monobank accounts")
	flagFirefly3ListAccounts := flag.Bool("firefly3-list-accounts", false, "list firefly3 accounts")

	flag.Parse()

	// manual transaction
	if *flagMonobankListAccounts {
		// get monobank accounts
		res, _, err := app.App().MonobankClient.Api.PersonalClientInfoGet(context.Background(), os.Getenv("MONOBANK_TOKEN"))
		if err != nil {
			log.Fatalln(err.Error())
		}

		// list accounts
		for _, row := range res.Accounts {
			fmt.Printf("%v\t%v\n", row.Id, (*row.MaskedPan)[0])
		}
	} else if *flagFirefly3ListAccounts {
		// get firefly3 accounts
		req := firefly3.AccountsApiListAccountOpts{
			Limit: optional.NewInt32(9999),
		}
		res, _, err := app.App().Firefly3Client.AccountsApi.ListAccount(context.Background(), &req)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}

		// list accounts
		for _, row := range res.Data {
			if row.Attributes.Active && (*row.Attributes.Type_ == firefly3.ASSET_ShortAccountTypeProperty) {
				fmt.Printf("%v\t%v\n", row.Id, row.Attributes.Name)
			}
		}
	} else if len(*flagMonobankDoTransaction) > 0 {
		var monobankTransaction monobank.WebHookResponse
		err := json.Unmarshal([]byte(*flagMonobankDoTransaction), &monobankTransaction)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}

		err = app.ImportTransaction(monobankTransaction)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
	} else {
		webhookLocalUrl := fmt.Sprintf("/webhook/%s", os.Getenv("MONOBANK_WEBHOOK_SECRET"))
		webhookUrl := fmt.Sprintf("https://%s/webhook/%s", os.Getenv("MONOBANK_WEBHOOK_DOMAIN"), os.Getenv("MONOBANK_WEBHOOK_SECRET"))

		// register monobank webhook
		_, err := app.App().MonobankClient.Api.PersonalWebhookPost(context.Background(), monobank.SetWebHook{WebHookUrl: webhookUrl}, os.Getenv("MONOBANK_TOKEN"))
		if err != nil {
			log.Fatalln("failed to register monobank webhook: " + err.Error())
		}

		// set webhook
		http.HandleFunc(webhookLocalUrl, handleWebhook)

		// health
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"pass"}`))
		})

		// listen server
		fmt.Println("webhook server listening on " + os.Getenv("LISTEN"))
		fmt.Println("webhook url " + webhookUrl)
		err = http.ListenAndServe(os.Getenv("LISTEN"), nil)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
	}
}
