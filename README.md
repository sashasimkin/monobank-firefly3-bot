# Monobank - Firefly3 bot

This bot is used to automatically log transactions from Monobank (via webhook) to [Firefly3](https://www.firefly-iii.org/).

Bot creates firefly3 transactions which are meant to be further processed by Firefly3 rules

## Installation

```sh
git clone https://gitea.stuzer.link/stuzer05/monobank-firefly3-bot.git

cd monobank-firefly3-bot

make
```

## Configuration

create `.env` file and `config.json`
```sh
cp .env.example .env
cp config.json.example config.json
```

set credential in `.env`

configure accounts in `config.json`

configure which transactions to match (by name or mcc codes) in `config.json`

## Run

you only need `.env`, `config.json` and build binary to run the bot

```sh
./monobank-firefly3-bot
```

bot will automatically register Monobank webhook url and start listening for incoming transactions

## Usage

to get monobank account ids use `--monobank-list-accounts` command
```sh
./monobank-firefly3-bot --monobank-list-accounts
0xzGO4sgEGXXXXXXqqSTJQ  537541******3946
wp6M2Ln7nkXXXXXXVYCCpA  444111******7344
4723djMLsLXXXXXXYjxqRw  444111******3747
```

to get firefly3 account ids use `--firefly3-list-accounts` command
```sh
./monobank-firefly3-bot --firefly3-list-accounts
1     Mono black
2     Wallet cash (UAH)
3     Mono white
4     PrivatBank virtual
```

## API docs
- https://api-docs.firefly-iii.org
- https://api.monobank.ua/docs/index.html