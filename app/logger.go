package app

import (
	"fmt"
	"os"
)

func LogString(str string) {
	if len(os.Getenv("LOG_FILE")) == 0 {
		return
	}

	f, err := os.OpenFile(os.Getenv("LOG_FILE"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
	}
	defer f.Close()

	if _, err := f.WriteString(str + "\n"); err != nil {
		fmt.Println(err)
	}
}
