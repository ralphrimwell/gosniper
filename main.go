package main

import (
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
)

func main() {
	err := SetupLogger()
	if err != nil {
		log.Fatal("Failed to setup loggers", err)
	}

	err = LoadConfig()
	if err != nil {
		log.Fatal("Failed to load config,", err)
	}

	tokens, err := ReadLines("tokens.txt")
	if err != nil {
		log.Fatal("Failed to read tokens", err)
	}
	InfoLogger.Println("Program has started...")

	sniper, err := NewSniper(tokens)
	if err != nil {
		log.Fatal("Failed to make sniper instance", err)
	}
	sniper.Start()

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	for {
		select {
		case <-sc:
			os.Exit(1)
		}

	}

}

func runCmd(name string, arg ...string) {
	cmd := exec.Command(name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func ClearTerminal() {
	switch runtime.GOOS {
	case "darwin":
		runCmd("clear")
	case "linux":
		runCmd("clear")
	case "windows":
		runCmd("cmd", "/c", "cls")
	default:
		runCmd("clear")
	}
}
