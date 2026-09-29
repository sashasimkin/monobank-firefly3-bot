package app

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestFireflyTransactionTransportOmitsZeroDates(t *testing.T) {
	var sentBody string
	transport := fireflyTransactionDateTransport{next: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		sentBody = string(body)
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Request:    request,
		}, nil
	})}
	client := &http.Client{Transport: transport}
	request, err := http.NewRequest(http.MethodPost, "http://firefly.local/api/v1/transactions", strings.NewReader(`{"transactions":[{"date":"2026-09-29T10:00:00Z","book_date":"0001-01-01T00:00:00Z"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if strings.Contains(sentBody, `"book_date"`) {
		t.Fatalf("transport sent zero optional date: %s", sentBody)
	}
	if !strings.Contains(sentBody, `"date"`) {
		t.Fatalf("transport removed required transaction date: %s", sentBody)
	}
}
