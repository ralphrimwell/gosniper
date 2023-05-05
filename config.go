package main

import (
	"io/ioutil"
	"log"

	"github.com/pelletier/go-toml"
)

var cfg MyConfig

type MyConfig struct {
	Token    string
	Cooldown float64
}

func LoadConfig() (err error) {

	file, err := ioutil.ReadFile("config.toml")
	if err != nil {
		log.Fatal("Failed to load config,", err)
		return err
	}

	err = toml.Unmarshal(file, &cfg)
	if err != nil {
		return err
	}

	return nil
}
