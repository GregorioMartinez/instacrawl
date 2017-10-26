package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
)

func getConfig(configPath string) (Config, error) {
	var config Config

	configFile, err := os.Open(configPath)
	if err != nil {
		return config, errors.New("unable to open config file")
	}
	defer configFile.Close()

	data, err := ioutil.ReadAll(configFile)
	if err != nil {
		return config, errors.New("unable to read config file")
	}

	err = json.Unmarshal(data, &config)
	if err != nil {
		return config, err
	}

	return config, nil
}

type Config struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
