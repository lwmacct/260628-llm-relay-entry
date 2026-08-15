package remotetoken

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
)

var Command = &cli.Command{
	Name:  "remote-token",
	Usage: "generate the fixed internal dp.22.remote token",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "resolver-url", Usage: "Vendor internal resolver URL", Required: true},
	},
	Action: action,
}

func action(_ context.Context, command *cli.Command) error {
	directiveSecret := strings.TrimSpace(os.Getenv("DIRECTIVE_TOKEN_SECRET"))
	resolverToken := strings.TrimSpace(os.Getenv("RELAY_ENTRY_S2S_TOKEN"))
	if directiveSecret == "" || resolverToken == "" {
		return errors.New("DIRECTIVE_TOKEN_SECRET and RELAY_ENTRY_S2S_TOKEN are required")
	}
	token, err := Generate(directiveSecret, command.String("resolver-url"), resolverToken)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, token)
	return err
}
