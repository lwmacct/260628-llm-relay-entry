package handler

import "github.com/danielgtaylor/huma/v2"

func utilHTTPConfig() huma.Config {
	cfg := huma.DefaultConfig("LLM Relay Entry API", "1.0.0")
	cfg.Servers = []*huma.Server{{URL: "/api"}}
	return cfg
}
