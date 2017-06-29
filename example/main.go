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

	log.Println("Starting v" + dshardmanager.VersionString)
	if FlagToken == "" {
		log.Fatal("No token specified")
	}

	if !strings.HasPrefix(FlagToken, "Bot ") {
		log.Fatal("dshardmanager only works on bot accounts, did you maybe forgot to add `Bot ` before the token?")
	}

	manager := dshardmanager.New(FlagToken)
	manager.Name = "ExampleBot"
	manager.LogChannel = FlagLogChannel
	manager.StatusMessageChannel = FlagLogChannel

	reccomended, err := manager.GetRecommendedCount()
	if err != nil {
		log.Fatal("Failed getting reccomended shard count")
	}
	if reccomended < 2 {
		manager.SetNumShards(5)
	}

	log.Println("Starting the shard manager")
	err = manager.Start()
	if err != nil {
		log.Fatal("Faled to start: ", err)
	}

	log.Println("Started!")
	select {}
}
