package main

import (
	"context"
	"fmt"

	"github.com/conductorone/baton-sdk/pkg/cli"
	"github.com/spf13/cobra"
)

// config defines the external configuration required for the connector to run.
type config struct {
	cli.BaseConfig `mapstructure:",squash"` // Puts the base config options in the same place as the connector options
	ProxyAddr      string                   `mapstructure:"proxyAddr"`
}

// validateConfig is run after the configuration is loaded, and should return an error if it isn't valid.
func validateConfig(ctx context.Context, cfg *config) error {
	// host and port of your teleport proxy service instance
	if cfg.ProxyAddr == "" {
		return fmt.Errorf("proxyAddr is required")
	}

	return nil
}

// cmdFlags sets the cmdFlags required for the connector.
func cmdFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("proxyAddr", "", "The fully-qualified teleport proxy service to connect with. Example: \"baton.teleport.sh:443\" ($BATON_PROXYADDR)")
}
