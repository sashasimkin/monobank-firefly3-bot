package app

import (
	"fmt"
	"gitea.stuzer.link/stuzer05/go-firefly3/v2"
	"gitea.stuzer.link/stuzer05/go-monobank"
	"os"
	"stuzer.link/monobank-firefly3-bot/config"
	"sync"
)

type Config struct {
	init           sync.Once
	Config         config.Config
	MonobankClient *monobank.APIClient
	Firefly3Client *firefly3.APIClient
}

var app Config

func App() *Config {
	return &app
}

func Init() {
	app.init.Do(func() {
		var err error

		// read config
		app.Config, err = config.Read(os.Getenv("CONFIG_PATH"))
		if err != nil {
			fmt.Println("cannot read config: " + err.Error())
			os.Exit(1)
		}

		// init monobank client
		monobankClientConf := monobank.NewConfiguration()
		app.MonobankClient = monobank.NewAPIClient(monobankClientConf)

		// init firefly3 client
		clientConf := firefly3.NewConfiguration()
		clientConf.BasePath = os.Getenv("FIREFLY3_API_URL")
		clientConf.AddDefaultHeader("Authorization", "Bearer "+os.Getenv("FIREFLY3_TOKEN"))
		app.Firefly3Client = firefly3.NewAPIClient(clientConf)
	})
}
