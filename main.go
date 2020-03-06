package main

import (
	"github.com/kelseyhightower/envconfig"

	"gitlab.com/prestrafe/prestrafe-gsi/gsi"
)

type ServerConfig struct {
	Addr string `default:"0.0.0.0"`
	Port int    `default:"8080"`
	Ttl  int    `default:"15"`
}

func main() {
	config := new(ServerConfig)
	envconfig.MustProcess("gsi", config)

	server := gsi.NewServer(config.Addr, config.Port, config.Ttl)
	if err := server.Start(); err != nil {
		panic(err)
	}
}
