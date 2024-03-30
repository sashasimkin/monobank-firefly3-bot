package main

import (
	"encoding/json"
	"io"
	monobank "main/monobank/api/webhook/models"
	"net/http"
)

func readResponseBody(r *http.Request) (monobank.Transaction, error) {
	// read body bytes
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return monobank.Transaction{}, err
	}

	//fmt.Println(string(body))
	//w.WriteHeader(http.StatusOK)
	//return

	body = []byte("{\"type\":\"StatementItem\",\"data\":{\"account\":\"4723djMLsLOCzhoeYjxqRw\",\"statementItem\":{\"id\":\"cSMr2xlFsfWPFeDLTg\",\"time\":1711806353,\"description\":\"Скасування. Bolt\",\"mcc\":4121,\"originalMcc\":4121,\"amount\":10300,\"operationAmount\":10300,\"currencyCode\":980,\"commissionRate\":0,\"cashbackAmount\":0,\"balance\":7974533,\"hold\":false}}}")
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
