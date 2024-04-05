package main

import (
	"encoding/json"
	"io"
	"net/http"
	monobank "stuzer.link/monobank-firefly3-bot/monobank/api/webhook/models"
)

func readResponseBody(r *http.Request) (monobank.Transaction, error) {
	// read body bytes
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return monobank.Transaction{}, err
	}

	LogString(string(body))

	// check empty body
	if len(string(body)) == 0 {
		return monobank.Transaction{}, err
	}

	// parse body
	var transaction monobank.Transaction
	err = json.Unmarshal(body, &transaction)
	if err != nil {
		return monobank.Transaction{}, err
	}

	return transaction, nil
}
