package main

import (
	"encoding/json"
	"errors"
	"gitea.stuzer.link/stuzer05/go-monobank"
	"io"
	"net/http"
)

func readRequestBody(r *http.Request) (monobank.WebHookResponse, error) {
	// read body bytes
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return monobank.WebHookResponse{}, err
	}

	LogString(string(body))

	// check empty body
	if len(string(body)) == 0 {
		return monobank.WebHookResponse{}, errors.New("empty body")
	}

	// parse body
	var transaction monobank.WebHookResponse
	err = json.Unmarshal(body, &transaction)
	if err != nil {
		return monobank.WebHookResponse{}, err
	}

	return transaction, nil
}
