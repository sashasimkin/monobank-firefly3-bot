package main

func Configure() ([]ShopConfigItem, map[string]string) {
	var ShopConfig []ShopConfigItem
	Firefy3AccountsConfig := make(map[string]string)

	/**
	 * Firefly3 accounts
	 */
	Firefy3AccountsConfig["Mono Black"] = "1"
	Firefy3AccountsConfig["Mono White"] = "60"
	Firefy3AccountsConfig["Wallet cash (UAH)"] = "4"

	/**
	 * Bills
	 */
	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Hetzner"},
		TransactionDescription: "Hetzner: vps2",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"YouTube"},
		TransactionDescription: "YouTube membership: Latte ASMR",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Київстар +380672463500"},
		TransactionDescription: "Kyivstar: +380672463500",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Lifecell +380732463500"},
		TransactionDescription: "Lifecell: +380732463500",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"JetBrains"},
		TransactionDescription: "JetBrains: GoLand",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Telegram"},
		TransactionDescription: "Telegram premium",
	})

	/**
	 * Credit payments
	 */
	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Платіж Tefal Initial+"},
		TransactionDescription: "Credit payment: TEFAL OptiGrill+ Initial GC706D34",
		TransactionDestination: "Credit: TEFAL OptiGrill+ Initial GC706D34",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Платіж Dyson"},
		TransactionDescription: "Credit payment: Dyson Supersonic HD07 Nickel/Copper",
		TransactionDestination: "Credit: Dyson Supersonic HD07 Nickel/Copper",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Платіж Pixel Watch 2"},
		TransactionDescription: "Credit payment: Google Pixel Watch 2 Wi-Fi",
		TransactionDestination: "Credit: Google Pixel Watch 2 Wi-Fi",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Платіж Apple Watch 9"},
		TransactionDescription: "Credit payment: Apple Watch Series 9",
		TransactionDestination: "Credit: Apple Watch Series 9",
	})

	/**
	 * People
	 */
	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Олена А."},
		TransactionDescription: "Doctor visit: Елена Ахрипова (психотерапевт)",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Ілля Ш."},
		TransactionDescription: "Legal services: Alva Privacy Law Firm",
		TransactionDestination: "Legal: Alva Privacy Law Firm",
		TransactionCategory:    "Legal services",
	})

	/**
	 * Other
	 */
	ShopConfig = append(ShopConfig, ShopConfigItem{
		MCCCodes:               []int{5411, 5499, 5451, 5422, 5412, 5921},
		Names:                  []string{"АТБ", "Велмарт", "Novus", "Glovo", "zakaz.ua", "Мегамаркет", "Сільпо", "Фора", "METRO"},
		TransactionDescription: "Groceries",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Аптека Доброго Дня", "Аптека оптових цін", "Аптека Копійка", "Аптека Гала", "Аптека АНЦ", "APTEKA 7", "Біла ромашка", "vidshkod ekv apt12", "Будемо Здорові Разом", "Apteka Ants", "Аптека Шар@"},
		TransactionDescription: "Medications",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		MCCCodes:               []int{4131, 4111, 4112},
		Names:                  []string{"Київ Цифровий", "Київпастранс"},
		TransactionDescription: "Public transport",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"McDonald’s"},
		TransactionDescription: "McDonalds",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"LeoCafe"},
		TransactionDescription: "Cafe",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Bolt food", "Glovo", "Menya Musashi"},
		TransactionDescription: "Restaurant",
	})

	ShopConfig = append(ShopConfig, ShopConfigItem{
		Names:                  []string{"Lumberjack Barberhouse"},
		TransactionDescription: "Lumberjack: haircut",
	})

	return ShopConfig, Firefy3AccountsConfig
}
