package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gitea.stuzer.link/stuzer05/go-firefly3/v2"
	"gitea.stuzer.link/stuzer05/go-monobank"
	"stuzer.link/monobank-firefly3-bot/config"
)

func TestOmitZeroOptionalFireflyDates(t *testing.T) {
	body := []byte(`{"apply_rules":true,"transactions":[{"date":"2026-09-29T10:00:00Z","amount":"12.34","category_name":"Groceries","external_id":"source-id","interest_date":"0001-01-01T00:00:00Z","book_date":"0001-01-01T00:00:00Z","process_date":"0001-01-01T00:00:00Z","due_date":"0001-01-01T00:00:00Z","payment_date":"0001-01-01T00:00:00Z","invoice_date":"0001-01-01T00:00:00Z"}]}`)

	got, changed, err := normalizeFireflyTransactionPayload(body)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("zero optional dates were not removed")
	}
	var payload struct {
		Transactions []map[string]json.RawMessage `json:"transactions"`
	}
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatal(err)
	}
	transaction := payload.Transactions[0]
	for _, field := range optionalFireflyDateFields {
		if _, ok := transaction[field]; ok {
			t.Errorf("zero optional date %q was not omitted", field)
		}
	}
	for _, field := range []string{"date", "amount", "category_name", "external_id"} {
		if _, ok := transaction[field]; !ok {
			t.Errorf("required transaction field %q was removed", field)
		}
	}
}

func TestKeepNonzeroOptionalFireflyDate(t *testing.T) {
	body := []byte(`{"transactions":[{"date":"2026-09-29T10:00:00Z","book_date":"2026-09-29T10:00:00Z","description":"Merchant"}]}`)
	got, changed, err := normalizeFireflyTransactionPayload(body)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("nonzero optional date unexpectedly changed the payload")
	}
	if string(got) != string(body) {
		t.Fatalf("payload changed: %s", got)
	}
}

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
	got := buildTransaction(monobank.StatementItemsInner{Id: "refund-id", Amount: 12345, Description: "Refund merchant"}, account, rule, true, monobankExternalID("account-id", "refund-id")+":refund", date)
	if got.Type_ == nil || *got.Type_ != firefly3.DEPOSIT_TransactionTypeProperty {
		t.Fatalf("refund type = %v, want deposit", got.Type_)
	}
	if got.Amount != "123.45" || got.SourceName != refundRevenue || got.DestinationName != account.Firefly3Name {
		t.Fatalf("unexpected refund transaction: amount=%q source=%q destination=%q", got.Amount, got.SourceName, got.DestinationName)
	}
	if got.CategoryName != "Groceries" || got.ExternalId != "monobank:account-id:refund-id:refund" {
		t.Fatalf("refund metadata = category %q external ID %q", got.CategoryName, got.ExternalId)
	}
}

func TestRefundRuleRequiresPositiveMonobankAmount(t *testing.T) {
	rules := []config.TransactionTypes{{NamesRefund: []string{"Refund merchant"}}}
	positive, isRefund := matchTransactionRuleFrom(rules, monobank.StatementItemsInner{Amount: 12345, Description: "Refund merchant"})
	if positive == nil || !isRefund {
		t.Fatal("positive refund entry did not match as a refund")
	}
	negative, isRefund := matchTransactionRuleFrom(rules, monobank.StatementItemsInner{Amount: -12345, Description: "Refund merchant"})
	if negative != nil || isRefund {
		t.Fatal("negative statement entry matched a refund-description rule")
	}
}

func TestRefundRulePrecedesBroadMCCRule(t *testing.T) {
	groceryRule := config.TransactionTypes{MccCodes: []int{5411}, Firefly3: config.TransactionTypeFirefly3{Category: "Groceries"}}
	refundRule := config.TransactionTypes{NamesRefund: []string{"Refund merchant"}, Firefly3: config.TransactionTypeFirefly3{Category: "Refunds"}}
	item := monobank.StatementItemsInner{Amount: 12345, Description: "Refund merchant", Mcc: 5411}

	got, isRefund := matchTransactionRuleFrom([]config.TransactionTypes{groceryRule, refundRule}, item)
	if got == nil || !isRefund || got.Firefly3.Category != "Refunds" {
		t.Fatalf("matching rule = %+v, refund = %t; want refund rule ahead of broad MCC rule", got, isRefund)
	}
}

func TestRuleOrderUsesFirstMatch(t *testing.T) {
	merchantRule := config.TransactionTypes{Names: []string{"Merchant"}, Firefly3: config.TransactionTypeFirefly3{Category: "Specific"}}
	mccRule := config.TransactionTypes{MccCodes: []int{5411}, Firefly3: config.TransactionTypeFirefly3{Category: "Groceries"}}
	item := monobank.StatementItemsInner{Amount: -1000, Description: "Merchant", Mcc: 5411}

	first, _ := matchTransactionRuleFrom([]config.TransactionTypes{merchantRule, mccRule}, item)
	if first == nil || first.Firefly3.Category != "Specific" {
		t.Fatalf("first matching rule = %+v, want specific merchant rule", first)
	}
	first, _ = matchTransactionRuleFrom([]config.TransactionTypes{mccRule, merchantRule}, item)
	if first == nil || first.Firefly3.Category != "Groceries" {
		t.Fatalf("first matching rule = %+v, want grocery MCC rule", first)
	}
}

func TestGroceryMCCBuildsCategorizedWithdrawal(t *testing.T) {
	account := config.Account{Firefly3Name: "Monobank Black UAH", Currency: "UAH"}
	rule := config.TransactionTypes{MccCodes: []int{5411, 5499, 5921}, Firefly3: config.TransactionTypeFirefly3{Type: "withdrawal", Category: "Groceries"}}
	tests := []struct {
		name string
		mcc  float64
	}{
		{name: "supermarket", mcc: 5411},
		{name: "specialty food store", mcc: 5499},
		{name: "liquor store", mcc: 5921},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := monobank.StatementItemsInner{Id: "purchase-id", Amount: -12345, Description: test.name, Mcc: test.mcc}
			matched, isRefund := matchTransactionRuleFrom([]config.TransactionTypes{rule}, item)
			if matched == nil || isRefund {
				t.Fatalf("MCC %.0f did not match the expense rule", test.mcc)
			}

			got := buildTransaction(item, account, matched, isRefund, "external-id", time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC))
			if got.Type_ == nil || *got.Type_ != firefly3.WITHDRAWAL_TransactionTypeProperty {
				t.Fatalf("transaction type = %v, want withdrawal", got.Type_)
			}
			if got.Amount != "123.45" || got.CategoryName != "Groceries" || got.SourceName != account.Firefly3Name {
				t.Fatalf("unexpected grocery transaction: amount=%q category=%q source=%q", got.Amount, got.CategoryName, got.SourceName)
			}
		})
	}
}

func TestMCC4829MatchesOnlyUniqueCrossAccountSameCurrencyAmounts(t *testing.T) {
	rule := config.TransactionTypes{MccCodes: []int{4829}, Firefly3: config.TransactionTypeFirefly3{Type: "transfer", Category: "Money transfers"}}
	out := statementEntry{Account: config.Account{MonobankId: "jar", Firefly3Name: "Jar", Currency: "UAH"}, Item: monobank.StatementItemsInner{Id: "out", Time: 1000, Amount: -10000, Mcc: 4829}}
	in := statementEntry{Account: config.Account{MonobankId: "card", Firefly3Name: "Card", Currency: "UAH"}, Item: monobank.StatementItemsInner{Id: "in", Time: 1050, Amount: 10000, Mcc: 4829}}
	pairs := matchStatementTransfers([]statementEntry{out, in}, []config.TransactionTypes{rule}, 120*time.Second)
	if len(pairs) != 1 || pairs[0].Outgoing.Item.Id != "out" || pairs[0].Incoming.Item.Id != "in" {
		t.Fatalf("matched pairs = %+v, want the unique outgoing/incoming pair", pairs)
	}

	otherIncoming := in
	otherIncoming.Item.Id = "second-in"
	otherIncoming.Item.Time = 1060
	pairs = matchStatementTransfers([]statementEntry{out, in, otherIncoming}, []config.TransactionTypes{rule}, 120*time.Second)
	if len(pairs) != 0 {
		t.Fatalf("ambiguous candidates matched as pairs: %+v", pairs)
	}
}

func TestMCC4829DoesNotPairDifferentCurrenciesOrAccounts(t *testing.T) {
	rule := config.TransactionTypes{MccCodes: []int{4829}, Firefly3: config.TransactionTypeFirefly3{Type: "transfer"}}
	out := statementEntry{Account: config.Account{MonobankId: "jar", Firefly3Name: "Jar", Currency: "UAH"}, Item: monobank.StatementItemsInner{Id: "out", Time: 1000, Amount: -10000, Mcc: 4829}}
	in := statementEntry{Account: config.Account{MonobankId: "card", Firefly3Name: "Card", Currency: "USD"}, Item: monobank.StatementItemsInner{Id: "in", Time: 1000, Amount: 10000, Mcc: 4829}}
	if pairs := matchStatementTransfers([]statementEntry{out, in}, []config.TransactionTypes{rule}, 120*time.Second); len(pairs) != 0 {
		t.Fatalf("different-currency entries matched: %+v", pairs)
	}
	in.Account = out.Account
	if pairs := matchStatementTransfers([]statementEntry{out, in}, []config.TransactionTypes{rule}, 120*time.Second); len(pairs) != 0 {
		t.Fatalf("same-account entries matched: %+v", pairs)
	}
}

func TestBuildStatementTransfer(t *testing.T) {
	out := statementEntry{Account: config.Account{MonobankId: "jar", Firefly3Name: "Jar", Currency: "UAH"}, Item: monobank.StatementItemsInner{Id: "out", Time: 1790672400, Amount: -12345, Mcc: 4829, Description: "Jar withdrawal"}}
	in := statementEntry{Account: config.Account{MonobankId: "card", Firefly3Name: "Card", Currency: "UAH"}, Item: monobank.StatementItemsInner{Id: "in", Time: 1790672401, Amount: 12345, Mcc: 4829}}
	got := buildStatementTransfer(statementTransferPair{Outgoing: out, Incoming: in})
	if got.Type_ == nil || *got.Type_ != firefly3.TRANSFER_TransactionTypeProperty {
		t.Fatalf("type = %v, want transfer", got.Type_)
	}
	if got.Amount != "123.45" || got.SourceName != "Jar" || got.DestinationName != "Card" || got.CategoryName != "" {
		t.Fatalf("unexpected transfer fields: amount=%q source=%q destination=%q category=%q", got.Amount, got.SourceName, got.DestinationName, got.CategoryName)
	}
	if got.ExternalId == "" || got.ExternalId != statementTransferExternalID(out, in) {
		t.Fatalf("transfer external ID is unstable: %q", got.ExternalId)
	}
}

func TestStatementDescriptionFallsBackForWhitespace(t *testing.T) {
	if got := statementDescription(" \t\n"); got != "Monobank account transfer" {
		t.Fatalf("statementDescription(whitespace) = %q, want non-empty fallback", got)
	}
}

func TestNormalizeFireflyDescriptionTrimsControlsAndLimitsLength(t *testing.T) {
	got := normalizeFireflyDescription("  first\nsecond\x00 ", "fallback")
	if got != "first second" {
		t.Fatalf("normalized description = %q, want whitespace and controls normalized", got)
	}
	long := strings.Repeat("я", fireflyDescriptionMaxLength+5)
	got = normalizeFireflyDescription(long, "fallback")
	if len([]rune(got)) != fireflyDescriptionMaxLength {
		t.Fatalf("normalized rune count = %d, want %d", len([]rune(got)), fireflyDescriptionMaxLength)
	}
}

func TestBuildTransactionFallsBackForWhitespaceDescription(t *testing.T) {
	account := config.Account{MonobankId: "account", Firefly3Name: "Account", Currency: "UAH"}
	item := monobank.StatementItemsInner{Id: "id", Time: 1790672400, Amount: -12345, Description: " \t\n"}
	got := buildTransaction(item, account, nil, false, "external-id", time.Unix(int64(item.Time), 0))
	if got.Description != "Monobank transaction" {
		t.Fatalf("transaction description = %q, want non-empty fallback", got.Description)
	}
}

func TestConfiguredTransferMatchWindow(t *testing.T) {
	if got, err := configuredTransferMatchWindow(config.Config{}); err != nil || got != 120*time.Second {
		t.Fatalf("default transfer match window = %s, %v; want 120s", got, err)
	}
	if _, err := configuredTransferMatchWindow(config.Config{TransferMatchWindowSeconds: 601}); err == nil {
		t.Fatal("accepted transfer match window above safe maximum")
	}
	if got, err := configuredTransferMatchWindow(config.Config{TransferMatchWindowSeconds: 300}); err != nil || got != 300*time.Second {
		t.Fatalf("configured transfer match window = %s, %v; want 300s", got, err)
	}
}

func TestNonGroceryMCCDoesNotMatchGroceryRule(t *testing.T) {
	rule := config.TransactionTypes{MccCodes: []int{5411}, Firefly3: config.TransactionTypeFirefly3{Category: "Groceries"}}
	got, isRefund := matchTransactionRuleFrom([]config.TransactionTypes{rule}, monobank.StatementItemsInner{Amount: -1000, Description: "Merchant", Mcc: 7997})
	if got != nil || isRefund {
		t.Fatalf("non-grocery MCC matched grocery rule: %+v", got)
	}
}

func TestUnmatchedWebhookRuleIsRetryable(t *testing.T) {
	if err := requireTransactionRule(nil, false, false); !errors.Is(err, errNoMatchingTransactionRule) {
		t.Fatalf("unmatched webhook rule error = %v, want retryable unmatched error", err)
	}
	if err := requireTransactionRule(nil, true, false); err != nil {
		t.Fatalf("statement poller should accept unmatched transactions for Uncategorized import: %v", err)
	}
	if err := requireTransactionRule(nil, false, true); err != nil {
		t.Fatalf("previously imported unmatched event should be acknowledged as a duplicate: %v", err)
	}
}

func TestFireflyValidationLoggingKeepsOnlyFieldNames(t *testing.T) {
	body := []byte(`{"message":"The given data was invalid.","errors":{"transactions.0.destination_name":["Sensitive merchant name is invalid"],"transactions.0.amount":["Private amount is invalid"]}}`)
	got := fireflyValidationFieldNames(body)
	want := []string{"transactions.0.amount", "transactions.0.destination_name"}
	if len(got) != len(want) {
		t.Fatalf("validation fields = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("validation fields = %v, want %v", got, want)
		}
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

func TestSyncWindowOverlapNeverPrecedesInitialCursor(t *testing.T) {
	const start = int64(10_000)
	if got := syncWindowStart(start+3600, start, 48*3600); got != start {
		t.Fatalf("overlap start = %d, want initial cursor %d", got, start)
	}
	if got := syncWindowStart(start+72*3600, start, 48*3600); got != start+24*3600 {
		t.Fatalf("overlap start = %d, want %d", got, start+24*3600)
	}
}
