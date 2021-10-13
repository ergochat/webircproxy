// Copyright (c) 2021 Shivaram Lingamneni <slingamn@cs.stanford.edu>
// released under the MIT license

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ergochat/webircproxy/irc"
)

// set via linker flags, either by make or by goreleaser:
var commit = ""  // git hash
var version = "" // tagged version

func main() {
	if len(os.Args) < 2 {
		log.Fatal("must pass config file as argument")
	}
	configfile := os.Args[1]
	config, err := irc.LoadConfig(configfile)
	if err != nil {
		log.Fatal("Config file did not load successfully: ", err.Error())
	}

	server, err := irc.NewServer(config)
	if err != nil {
		log.Fatal(fmt.Sprintf("Could not load server: %s", err.Error()))
	}
	server.Run()
}
