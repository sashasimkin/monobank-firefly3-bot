package config

func (c *Config) GetAccountByMonobankId(q string) ConfigAccount {
	for _, row := range c.Accounts {
		if row.MonobankId == q {
			return row
		}
	}

	return ConfigAccount{}
}

func (c *Config) GetAccountByFirefly3Name(q string) ConfigAccount {
	for _, row := range c.Accounts {
		if row.Firefly3Name == q {
			return row
		}
	}

	return ConfigAccount{}
}
