package app

import (
	"testing"
	"time"

	"gitea.stuzer.link/stuzer05/go-firefly3/v2"
	"gitea.stuzer.link/stuzer05/go-monobank"
	"stuzer.link/monobank-firefly3-bot/config"
)

func TestFormatMinorAmountKeepsCents(t *testing.T) {
	for input, want := range map[int64]string{0: "0.00", 468: "4.68", 168443: "1684.43", -512711: "5127.11"} {
		if got := formatMinorAmount(input); got != want {
			t.Errorf("formatMinorAmount(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestUnmatchedStatementBecomesUncategorizedInflowOrOutflow(t *testing.T) {
	account := config.Account{Firefly3Name: "Monobank Black UAH", Currency: "UAH"}
	date := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name            string
		amount          float64
		wantType        firefly3.TransactionTypeProperty
		wantAmount      string
		wantSource      string
		wantDestination string
	}{
		{"purchase", -12345, firefly3.WITHDRAWAL_TransactionTypeProperty, "123.45", account.Firefly3Name, uncategorizedExpense},
		{"incoming", 500, firefly3.DEPOSIT_TransactionTypeProperty, "5.00", uncategorizedIncome, account.Firefly3Name},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := buildTransaction(monobank.StatementItemsInner{Id: "source-id", Amount: test.amount, Description: "Merchant"}, account, nil, false, "external-id", date)
			if got.Amount != test.wantAmount {
				t.Fatalf("amount = %s, want %s", got.Amount, test.wantAmount)
			}
			if got.Type_ == nil || *got.Type_ != test.wantType {
				t.Fatalf("type = %v, want %s", got.Type_, test.wantType)
			}
			if got.SourceName != test.wantSource || got.DestinationName != test.wantDestination {
				t.Fatalf("accounts = %q -> %q, want %q -> %q", got.SourceName, got.DestinationName, test.wantSource, test.wantDestination)
			}
			if got.CategoryName != uncategorizedCategory || got.ExternalId != "external-id" {
				t.Fatalf("missing category or source ID: %+v", got)
			}
		})
	}
}

func TestRefundBuildsSeparateDeposit(t *testing.T) {
	account := config.Account{Firefly3Name: "Monobank Black UAH", Currency: "UAH"}
	date := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	rule := &config.TransactionTypes{Firefly3: config.TransactionTypeFirefly3{Category: "Groceries"}}
	got := buildTransaction(monobank.StatementItemsInner{Id: "refund-id", Amount: 12345, Description: "Refund merchant"}, account, rule, true, "refund-external-id", date)
	if got.Type_ == nil || *got.Type_ != firefly3.DEPOSIT_TransactionTypeProperty {
		t.Fatalf("refund type = %v, want deposit", got.Type_)
	}
	if got.Amount != "123.45" || got.SourceName != refundRevenue || got.DestinationName != account.Firefly3Name {
		t.Fatalf("unexpected refund transaction: amount=%q source=%q destination=%q", got.Amount, got.SourceName, got.DestinationName)
	}
	if got.CategoryName != "Groceries" || got.ExternalId != "refund-external-id" {
		t.Fatalf("refund metadata = category %q external ID %q", got.CategoryName, got.ExternalId)
	}
}

func TestSyncStateRoundTrip(t *testing.T) {
	path := t.TempDir() + "/nested/state.json"
	want := SyncState{LastSync: map[string]int64{"account-a": 1790672400}}
	if err := writeSyncState(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := readSyncState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastSync["account-a"] != want.LastSync["account-a"] {
		t.Fatalf("state cursor = %d, want %d", got.LastSync["account-a"], want.LastSync["account-a"])
	}
}
