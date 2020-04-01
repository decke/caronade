package main

import (
	"flag"

	"github.com/decke/caronade/internal/server"
)

func main() {
	var cfgfile string

	flag.StringVar(&cfgfile, "config", "caronade.yaml", "Path to config file")
	flag.Parse()

	server.StartServer(cfgfile)
}
