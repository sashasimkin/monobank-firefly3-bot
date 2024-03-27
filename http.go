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

	//body = []byte("{\"type\":\"StatementItem\",\"data\":{\"account\":\"4723djMLsLOCzhoeYjxqRw\",\"statementItem\":{\"id\":\"5_NQ0arGAmp2pyNzvA\",\"time\":1711544958,\"description\":\"З чорної картки\",\"mcc\":4829,\"originalMcc\":4829,\"amount\":-572000,\"operationAmount\":-572000,\"currencyCode\":980,\"commissionRate\":22000,\"cashbackAmount\":0,\"balance\":8101246,\"hold\":true,\"receiptId\":\"EMXC-P266-90PC-EB8C\"}}}")
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
