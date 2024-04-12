package main

import (
	"encoding/json"
	"gitea.stuzer.link/stuzer05/go-monobank"
	"io"
	"net/http"
	"stuzer.link/monobank-firefly3-bot/app"
)

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	// read request body bytes
	body, err := io.ReadAll(r.Body)
	if err != nil {
		app.LogString(err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	// log body string
	app.LogString(string(body))

	// check request empty body
	if len(string(body)) == 0 {
		app.LogString("empty body")
		w.WriteHeader(http.StatusOK)
		return
	}

	// parse request body
	var monobankTransaction monobank.WebHookResponse
	err = json.Unmarshal(body, &monobankTransaction)
	if err != nil {
		app.LogString(err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	err = app.ImportTransaction(monobankTransaction)
	if err != nil {
		app.LogString(err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
}
