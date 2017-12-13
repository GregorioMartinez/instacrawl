package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
)

func getConfig(configPath string) (config, error) {
	var conf config

	configFile, err := os.Open(configPath)
	if err != nil {
		return conf, errors.New("unable to open config file")
	}

	defer func() {
		if err1 := configFile.Close(); err1 != nil {
			err = err1
		}
	}()

	data, err := ioutil.ReadAll(configFile)
	if err != nil {
		return conf, errors.New("unable to read config file")
	}

	err = json.Unmarshal(data, &conf)
	return conf, err
}

type config struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	NeoAuth   string `json:"neo_auth"`
	MySQLAuth string `json:"mysql_auth"`
}
