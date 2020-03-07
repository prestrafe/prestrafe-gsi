package main

import (
	"github.com/kelseyhightower/envconfig"

	"gitlab.com/prestrafe/prestrafe-gsi/server"
)

type ServerConfig struct {
	Addr string `default:"0.0.0.0"`
	Port int    `default:"8080"`
	Ttl  int    `default:"15"`
}

func main() {
	config := new(ServerConfig)
	envconfig.MustProcess("gsi", config)

	gsiServer := server.New(config.Addr, config.Port, config.Ttl, &server.ToggleTokenFilter{Value: true})
	if err := gsiServer.Start(); err != nil {
		panic(err)
	}
}
