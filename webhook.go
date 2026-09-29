package main

import (
	"encoding/json"
	"gitea.stuzer.link/stuzer05/go-monobank"
	"io"
	"log"
	"net/http"
	"stuzer.link/monobank-firefly3-bot/app"
)

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("read Monobank webhook body: %v", err)
		http.Error(w, "unable to read webhook", http.StatusInternalServerError)
		return
	}
	if len(body) == 0 {
		http.Error(w, "empty webhook", http.StatusBadRequest)
		return
	}

	var transaction monobank.WebHookResponse
	if err := json.Unmarshal(body, &transaction); err != nil {
		log.Printf("parse Monobank webhook: %v", err)
		http.Error(w, "invalid webhook", http.StatusBadRequest)
		return
	}
	if err := app.ImportTransaction(transaction); err != nil {
		log.Printf("import Monobank webhook: %v", err)
		http.Error(w, "transaction import failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
