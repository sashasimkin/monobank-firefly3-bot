package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"net/http"
	"os"
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

	// set webhook
	http.HandleFunc("/webhook", handleWebhook)

	// listen server
	fmt.Println("Webhook server listening on :3021") // @todo make env
	err = http.ListenAndServe(":3021", nil)
	if err != nil {
		panic(err.Error())
	}
}
