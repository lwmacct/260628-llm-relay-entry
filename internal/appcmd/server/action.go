package server

import (
	"context"

	"github.com/lwmacct/260628-llm-relay-entry/internal/config"
	"github.com/urfave/cli/v3"
)

func action(ctx context.Context, _ *cli.Command, cfg *config.Config) error {
	return NewApp(cfg).Run(ctx)
}
