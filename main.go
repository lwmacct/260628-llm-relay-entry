package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/lwmacct/251207-go-pkg-version/pkg/version"
	"github.com/lwmacct/251219-go-pkg-logm/pkg/logm"
	"github.com/lwmacct/260628-llm-relay-entry/internal/appcmd/remotetoken"
	"github.com/lwmacct/260628-llm-relay-entry/internal/appcmd/server"
	"github.com/lwmacct/260628-llm-relay-entry/internal/config"
	"github.com/urfave/cli/v3"
)

func main() {
	logm.MustInit(logm.PresetAuto())

	cmd := &cli.Command{
		Name:            "app",
		Usage:           "LLM relay entry service",
		Version:         version.AppVersion,
		Commands:        []*cli.Command{server.Command, remotetoken.Command, version.Command},
		HideHelpCommand: true,
		Action: func(ctx context.Context, c *cli.Command) error {
			return cli.ShowSubcommandHelp(c)
		},
	}
	config.Manager.MustConfigure(cmd)

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}
