package main

import (
	"flag"
	"fmt"
	"gitea.stuzer.link/stuzer05/go-monobank"
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
	flagDoTransaction := flag.String("monobank-transaction", "", "run monobank transaction JSON manually")

	flag.Parse()

	// manual transaction
	if len(*flagDoTransaction) > 0 {
		w := httptest.NewRecorder()

		r := &http.Request{
			Method: http.MethodPost,
			Header: make(http.Header),
			Body:   io.NopCloser(strings.NewReader(*flagDoTransaction)),
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
		monobankClientConf := monobank.NewConfiguration()
		monobankClient := monobank.NewAPIClient(monobankClientConf)

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
