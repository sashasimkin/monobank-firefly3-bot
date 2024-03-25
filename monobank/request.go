package monobank

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
)

func Request(method string, url string, data interface{}) (string, error) {
	apiMonabankToken := os.Getenv("MONOBANK_TOKEN")

	json, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	r, err := http.NewRequest(method, url, bytes.NewBuffer(json))
	if err != nil {
		return "", err
	}

	r.Header.Add("Accept", "application/vnd.firefly3+json")
	r.Header.Add("Content-Type", "application/json")
	r.Header.Add("X-Token", apiMonabankToken)

	client := &http.Client{}
	res, err := client.Do(r)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	return string(body), nil
}
