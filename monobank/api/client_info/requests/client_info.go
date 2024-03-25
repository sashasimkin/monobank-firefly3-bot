package requests

import (
	"encoding/json"
	"main/monobank"
	models2 "main/monobank/api/client_info/models"
)

func ClientInfo() (models2.ClientInfo, error) {
	data := models2.ClientInfo{}

	responseJson, err := monobank.Request("GET", "https://firefly3.monobank.ua/personal/client-info", struct{}{})
	if err != nil {
		return data, err
	}
	json.Unmarshal([]byte(responseJson), &data)

	return data, nil
}
