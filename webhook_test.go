package main

import (
	"gitea.stuzer.link/stuzer05/go-firefly3/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"stuzer.link/monobank-firefly3-bot/app"
	"stuzer.link/monobank-firefly3-bot/config"
)

func TestWebhookReturnsServerErrorForUnmatchedTransaction(t *testing.T) {
	previous := app.App().Config
	previousClient := app.App().Firefly3Client
	app.App().Config = config.Config{
		Accounts: []config.Account{{Firefly3Name: "Checking", MonobankId: "account-id"}},
	}
	t.Cleanup(func() { app.App().Config = previous })
	t.Cleanup(func() { app.App().Firefly3Client = previousClient })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[],"meta":{"pagination":{"total_pages":1}}}`))
	}))
	defer server.Close()
	clientConfig := firefly3.NewConfiguration()
	clientConfig.BasePath = server.URL + "/api"
	app.App().Firefly3Client = firefly3.NewAPIClient(clientConfig)

	body := `{"data":{"account":"account-id","statementItem":{"id":"txn-id","time":1790672400,"amount":-100,"description":"Merchant"}}}`
	request := httptest.NewRequest(http.MethodPost, "/webhook/test-secret", strings.NewReader(body))
	response := httptest.NewRecorder()

	handleWebhook(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("webhook status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}
