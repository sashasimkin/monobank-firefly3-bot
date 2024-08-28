package config

func (c *Config) GetAccountByMonobankId(q string) Account {
	for _, row := range c.Accounts {
		if row.MonobankId == q {
			return row
		}
	}

	return Account{}
}

func (c *Config) GetAccountByFirefly3Name(q string) Account {
	for _, row := range c.Accounts {
		if row.Firefly3Name == q {
			return row
		}
	}

	return Account{}
}
