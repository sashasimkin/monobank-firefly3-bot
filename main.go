package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"gitea.stuzer.link/stuzer05/go-firefly3/v2"
	"gitea.stuzer.link/stuzer05/go-monobank"
	"github.com/antihax/optional"
	"github.com/joho/godotenv"
	"log"
	"os"
	"stuzer.link/monobank-firefly3-bot/app"
	"time"
)

func main() {
	// load .env
	godotenv.Load(".env")

	// init app
	app.Init()

	// flags
	flagMonobankDoTransaction := flag.String("monobank-transaction", "", "run monobank transaction JSON manually")
	flagMonobankListAccounts := flag.Bool("monobank-list-accounts", false, "list monobank accounts")
	flagFirefly3ListAccounts := flag.Bool("firefly3-list-accounts", false, "list firefly3 accounts")
	flagMonobankSync := flag.Bool("monobank-sync", false, "sync recent Monobank statements incrementally")
	flagMonobankImportHistory := flag.String("monobank-import-history", "", "import statement history from YYYY-MM-DD (requires configured transaction rules)")

	flag.Parse()

	// manual transaction
	if *flagMonobankListAccounts {
		// get monobank accounts
		res, _, err := app.App().MonobankClient.Api.PersonalClientInfoGet(context.Background(), os.Getenv("MONOBANK_TOKEN"))
		if err != nil {
			log.Fatalln(err.Error())
		}

		// list accounts
		for _, row := range res.Accounts {
			fmt.Printf("%v\t%v\n", row.Id, (*row.MaskedPan)[0])
		}
	} else if *flagFirefly3ListAccounts {
		// get firefly3 accounts
		req := firefly3.AccountsApiListAccountOpts{
			Limit: optional.NewInt32(9999),
		}
		res, _, err := app.App().Firefly3Client.AccountsApi.ListAccount(context.Background(), &req)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}

		// list accounts
		for _, row := range res.Data {
			if row.Attributes.Active && (*row.Attributes.Type_ == firefly3.ASSET_ShortAccountTypeProperty) {
				fmt.Printf("%v\t%v\n", row.Id, row.Attributes.Name)
			}
		}
	} else if *flagMonobankSync {
		if err := app.SyncMonobankTransactions(context.Background()); err != nil {
			log.Fatalln("Monobank sync failed: " + err.Error())
		}
	} else if *flagMonobankImportHistory != "" {
		from, err := time.Parse("2006-01-02", *flagMonobankImportHistory)
		if err != nil {
			log.Fatalln("history start must use YYYY-MM-DD")
		}
		if err := app.ImportMonobankHistory(context.Background(), from); err != nil {
			log.Fatalln("Monobank history import failed: " + err.Error())
		}
	} else if len(*flagMonobankDoTransaction) > 0 {
		var monobankTransaction monobank.WebHookResponse
		err := json.Unmarshal([]byte(*flagMonobankDoTransaction), &monobankTransaction)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}

		err = app.ImportTransaction(monobankTransaction)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
	} else {
		if err := runWebhookServer(); err != nil {
			log.Fatal(err)
		}
	}
}
