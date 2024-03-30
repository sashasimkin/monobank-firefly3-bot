package main

import (
	"flag"
	"fmt"
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

// curl -X POST https://api.monobank.ua/personal/webhook -H 'Content-TransactionType: application/json' -H 'X-Token: ' -d '{"webHookUrl":"https://monobank-firefly3.stuzer.link/webhook"}'

// curl -X POST https://monobank-firefly3.stuzer.link/webhook -H 'Content-TransactionType: application/json' -d '{"test":123}'

func main() {
	// load .env
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	// test config read
	_, err = ReadConfig(os.Getenv("CONFIG_PATH"))
	if err != nil {
		fmt.Println("cannot read config - " + err.Error())
		return
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
		// set webhook
		http.HandleFunc("/webhook", handleWebhook)

		// listen server
		fmt.Println("Webhook server listening on " + os.Getenv("LISTEN"))
		err = http.ListenAndServe(os.Getenv("LISTEN"), nil)
		if err != nil {
			panic(err.Error())
		}
	}
}
