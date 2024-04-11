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

