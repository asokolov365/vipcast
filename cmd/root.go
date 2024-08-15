// Copyright 2024 Andrew Sokolov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package cmd implements a CLI command.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/asokolov365/snakecharmer"
	"github.com/asokolov365/vipcast/app"
	"github.com/asokolov365/vipcast/config"
	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/asokolov365/vipcast/version"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var logger *zerolog.Logger

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "vipcast",
	Short: "*vipcast* injects client application Virtual IP address via BGP protocol.",
	// Long:  `A longer description that spans multiple lines and likely contains`,

	// examples and usage of using your application. For example:
	// Example: `Some example`,

	// Disable automatic printing of usage information whenever an error
	// occurs. Many errors are not the result of a bad command invocation,
	// e.g. attempting to start a node on an in-use port, and printing the
	// usage information in these cases obscures the cause of the error.
	// Commands should manually print usage information when the error is,
	// in fact, a result of a bad invocation, e.g. too many arguments.
	SilenceUsage: true,
	PreRunE: func(cmd *cobra.Command, args []string) (err error) {
		if len(configFile) > 0 {
			err = charmer.Set(
				snakecharmer.WithConfigFilePath(configFile),
				snakecharmer.WithConfigFileType("yaml"),
			)
			if err != nil {
				return err
			}
		}

		// cmd.DebugFlags()
		// This fills out the Config struct.
		if err = charmer.UnmarshalExact(); err != nil {
			if errUsage := cmd.Usage(); errUsage != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", errUsage.Error())
			}
			return err
		}

		if err = logging.Init(config.AppConfig.Logging, os.Stderr); err != nil {
			return err
		}
		logger = logging.GetSubLogger("root")
		if err = app.Init(); err != nil {
			return err
		}
		if *config.AppConfig.Consul.ClientSDInterval < 10 {
			logger.Warn().Int("interval", *config.AppConfig.Consul.ClientSDInterval).
				Msg("consul.client-discovery-interval is too short, setting it to 10")
			*config.AppConfig.Consul.ClientSDInterval = 10
		}
		if *config.AppConfig.MonitorInterval < 5 {
			logger.Warn().Int("interval", *config.AppConfig.Consul.ClientSDInterval).
				Msg("monitor-interval is too short, setting it to 5")
			*config.AppConfig.MonitorInterval = 5
		}

		return nil
	},

	RunE: func(cmd *cobra.Command, args []string) (err error) {
		// Create a context that cancels when OS signals come in.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
		defer stop()

		if err := app.Run(ctx); err != nil {
			return err
		}

		return nil
	},

	Version: fmt.Sprintf("Version: %s\n", version.Version),
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		return 1
	}
	return 0
}

var (
	vpr        *viper.Viper
	charmer    *snakecharmer.SnakeCharmer
	configFile string
)

func init() {
	var err error
	rootCmd.SetVersionTemplate(`{{printf "vipcast %s" .Version}}`)
	vpr = viper.New()
	config.AppConfig = config.DefaultConfig()

	charmer, err = snakecharmer.NewSnakeCharmer(
		snakecharmer.WithCobraCommand(rootCmd),
		snakecharmer.WithViper(vpr),
		snakecharmer.WithResultStruct(config.AppConfig),
	)

	if err != nil {
		panic(fmt.Sprintf("error init SnakeCharmer: %s", err.Error()))
	}

	rootCmd.PersistentFlags().StringVarP(
		&configFile, "config", "c", configFile, "Path to a vipcast config file")
	// This adds Flags automatically generated from the config.AppConfig struct
	charmer.AddFlags()
}
