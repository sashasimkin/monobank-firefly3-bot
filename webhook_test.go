package main

import (
	"gitea.stuzer.link/stuzer05/go-firefly3/v2"
	"gitea.stuzer.link/stuzer05/go-monobank"
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

func TestWebhookAcknowledgesAndSchedulesInternalTransfer(t *testing.T) {
	previousConfig := app.App().Config
	previousSchedule := scheduleInternalTransferWebhook
	app.App().Config = config.Config{
		Accounts: []config.Account{{Firefly3Name: "Checking", MonobankId: "account-id", Currency: "UAH"}},
		TransactionTypes: []config.TransactionTypes{{
			MccCodes: []int{4829},
			Firefly3: config.TransactionTypeFirefly3{Type: "transfer"},
		}},
	}
	t.Cleanup(func() { app.App().Config = previousConfig })
	t.Cleanup(func() { scheduleInternalTransferWebhook = previousSchedule })

	scheduled := false
	scheduleInternalTransferWebhook = func(event monobank.WebHookResponse) error {
		scheduled = event.Data.StatementItem.Id == "txn-id"
		return nil
	}
	body := `{"data":{"account":"account-id","statementItem":{"id":"txn-id","time":1790672400,"amount":-100,"mcc":4829,"description":"Transfer"}}}`
	request := httptest.NewRequest(http.MethodPost, "/webhook/test-secret", strings.NewReader(body))
	response := httptest.NewRecorder()

	handleWebhook(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("webhook status = %d, want %d", response.Code, http.StatusOK)
	}
	if !scheduled {
		t.Fatal("internal transfer was not scheduled for counterpart lookup")
	}
}
