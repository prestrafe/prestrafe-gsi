package main

import (
	"fmt"
	"net/http"

	"github.com/kelseyhightower/envconfig"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"gitlab.com/prestrafe/prestrafe-gsi/server"
)

type ServerConfig struct {
	Addr       string `default:""`
	Port       int    `default:"8080"`
	MetricPort int    `default:"9080"`
	Ttl        int    `default:"15"`
}

func main() {
	config := new(ServerConfig)
	envconfig.MustProcess("gsi", config)

	http.Handle("/metrics", promhttp.Handler())
	go func() {
		_ = http.ListenAndServe(fmt.Sprintf(":%d", config.MetricPort), nil)
	}()

	gsiServer := server.New(config.Addr, config.Port, config.Ttl, &server.ToggleTokenFilter{Value: true})
	if err := gsiServer.Start(); err != nil {
		panic(err)
	}
}
