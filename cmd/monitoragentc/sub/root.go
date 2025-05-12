// Copyright 2018 vpp_team, vpp_team@gmail.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sub

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"monitoragent/client"
	"monitoragent/pkg/util/version"
)

var (
	// cfgFile     string
	// cfgDir      string
	showVersion bool
)

func init() {
	// rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "./monitoragentc.ini", "config file of monitoragentc")
	// rootCmd.PersistentFlags().StringVarP(&cfgDir, "config_dir", "", "", "config directory, run one monitoragentc service for each file in config directory")
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "version of monitoragentc")
}

var rootCmd = &cobra.Command{
	Use:   "monitoragentc",
	Short: "monitoragentc is the client of monitoragent (https://monitoragent)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if showVersion {
			fmt.Println(version.Full())
			return nil
		}

		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func handleTermSignal(svr *client.Service) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	svr.GracefulClose(500 * time.Millisecond)
}

// func startService(
// 	cfg *v1.ClientCommonConfig,
// 	forwardCfgs []v1.ForwardConfigurer,
// 	visitorCfgs []v1.VisitorConfigurer,
// 	cfgFile string,
// ) error {
// 	log.InitLog(cfg.Log.To, cfg.Log.Level, cfg.Log.MaxDays, cfg.Log.DisablePrintColor)

// 	if cfgFile != "" {
// 		log.Info("start monitoragentc service for config file [%s]", cfgFile)
// 		defer log.Info("monitoragentc service for config file [%s] stopped", cfgFile)
// 	}

// 	svc, err := client.NewService(client.ServiceOptions{
// 		Common:         cfg,
// 		ForwardCfgs:    forwardCfgs,
// 		VisitorCfgs:    visitorCfgs,
// 		ConfigFilePath: cfgFile,
// 	})
// 	if err != nil {
// 		return err
// 	}

// 	shouldGracefulClose := cfg.Transport.Protocol == "kcp" || cfg.Transport.Protocol == "quic"
// 	// Capture the exit signal if we use kcp or quic.
// 	if shouldGracefulClose {
// 		go handleTermSignal(svc)
// 	}

// 	return svc.Run(context.Background())
// }
