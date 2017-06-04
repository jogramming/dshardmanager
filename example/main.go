package main

import (
	"flag"
	"github.com/jonas747/dshardmanager"
	"log"
	"strings"
)

var (
	FlagToken      string
	FlagLogChannel string
)

func main() {

	flag.StringVar(&FlagToken, "t", "", "Discord token")
	flag.StringVar(&FlagLogChannel, "c", "", "Log channel, optional")
	flag.Parse()

	log.Println("Starting...")
	if FlagToken == "" {
		log.Fatal("No token specified")
	}

	if !strings.HasPrefix(FlagToken, "Bot ") {
		log.Fatal("dshardmanager only works on bot accounts, did you maybe forgot to add `Bot ` before the token?")
	}

	manager := dshardmanager.New(FlagToken,
		dshardmanager.OptLogChannel(FlagLogChannel), dshardmanager.OptLogEventsToDiscord(true, true))

	log.Println("Starting the shard manager")
	err := manager.Start()
	if err != nil {
		log.Fatal("Faled to start: ", err)
	}

	log.Println("Started!")
	select {}
}
