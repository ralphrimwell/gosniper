package main

import (
	"log"
	"os"
	"path/filepath"
)

var (
	WarningLogger *log.Logger
	InfoLogger    *log.Logger
	ErrorLogger   *log.Logger
	SuccessLogger *log.Logger
)

func SetupLogger() (err error) {
	newpath := filepath.Join(".", "logs")
	err = os.MkdirAll(newpath, os.ModePerm)
	if err != nil {
		log.Fatal(err)
		return err
	}

	file, err := os.OpenFile("logs/.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
		return err

	}

	InfoLogger = log.New(file, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	WarningLogger = log.New(file, "WARNING: ", log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLogger = log.New(file, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
	SuccessLogger = log.New(file, "SUCCESS: ", log.Ldate|log.Ltime|log.Lshortfile)
	return nil
}
