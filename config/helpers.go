package config

func (c *Config) GetAccountByMonobankId(q string) ConfigAccount {
	for _, row := range c.Accounts {
		if row.MonobankId == q {
			return row
		}
	}

	return ConfigAccount{}
}
