package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const zeroFireflyDate = "0001-01-01T00:00:00Z"

var optionalFireflyDateFields = [...]string{
	"interest_date",
	"book_date",
	"process_date",
	"due_date",
	"payment_date",
	"invoice_date",
}

// The Firefly SDK models optional dates as time.Time. encoding/json serializes
// their zero values despite omitempty, and Firefly rejects year-one dates.
// Omit only those zero-valued fields from transaction-store requests.
type fireflyTransactionDateTransport struct {
	next http.RoundTripper
}

func newFireflyTransactionHTTPClient() *http.Client {
	return &http.Client{Transport: fireflyTransactionDateTransport{next: http.DefaultTransport}}
}

func (t fireflyTransactionDateTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/transactions") || request.Body == nil {
		return t.next.RoundTrip(request)
	}

	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	_ = request.Body.Close()

	filtered, changed, err := omitZeroOptionalFireflyDates(body)
	if err != nil {
		return nil, err
	}
	if !changed {
		filtered = body
	}

	request = request.Clone(request.Context())
	request.Body = io.NopCloser(bytes.NewReader(filtered))
	request.ContentLength = int64(len(filtered))
	request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(filtered)), nil
	}
	return t.next.RoundTrip(request)
}

func omitZeroOptionalFireflyDates(body []byte) ([]byte, bool, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false, err
	}

	transactionsJSON, ok := payload["transactions"]
	if !ok {
		return body, false, nil
	}
	var transactions []map[string]json.RawMessage
	if err := json.Unmarshal(transactionsJSON, &transactions); err != nil {
		return nil, false, err
	}

	changed := false
	for _, transaction := range transactions {
		for _, field := range optionalFireflyDateFields {
			valueJSON, ok := transaction[field]
			if !ok {
				continue
			}
			var value string
			if json.Unmarshal(valueJSON, &value) == nil && value == zeroFireflyDate {
				delete(transaction, field)
				changed = true
			}
		}
	}
	if !changed {
		return body, false, nil
	}

	transactionsJSON, err := json.Marshal(transactions)
	if err != nil {
		return nil, false, err
	}
	payload["transactions"] = transactionsJSON
	filtered, err := json.Marshal(payload)
	if err != nil {
		return nil, false, err
	}
	return filtered, true, nil
}
