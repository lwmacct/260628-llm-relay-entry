package server

import (
	"github.com/urfave/cli/v3"

	"github.com/lwmacct/251207-go-pkg-version/pkg/version"
	"github.com/lwmacct/260628-llm-relay-entry/internal/config"
)

var Command = &cli.Command{
	Name: "server", Usage: "start API entry server", Action: config.Manager.Action(action),
	Commands: []*cli.Command{version.Command}, HideHelpCommand: true,
}
