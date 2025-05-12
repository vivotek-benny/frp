// Copyright 2023 The monitoragent Authors
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
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"monitoragent/client"
	"monitoragent/pkg/config"
	v1 "monitoragent/pkg/config/v1"
	"monitoragent/pkg/config/v1/validation"
	"monitoragent/pkg/util/log"
)

var forwardTypes = []v1.ForwardType{
	v1.ForwardTypeTCP,
	v1.ForwardTypeUDP,
	v1.ForwardTypeTCPMUX,
	v1.ForwardTypeHTTP,
	v1.ForwardTypeHTTPS,
	v1.ForwardTypeSTCP,
	v1.ForwardTypeSUDP,
	v1.ForwardTypeXTCP,
}

func init() {
	for _, typ := range forwardTypes {
		c := v1.NewForwardConfigurerByType(typ)
		if c == nil {
			panic("forward type: " + typ + " not support")
		}
		clientCfg := v1.ClientCommonConfig{}
		cmd := NewForwardCommand(string(typ), c, &clientCfg)
		config.RegisterClientCommonConfigFlags(cmd, &clientCfg)
		config.RegisterForwardFlags(cmd, c)

		// add sub command for visitor
		// if lo.Contains(visitorTypes, v1.VisitorType(typ)) {
		// 	vc := v1.NewVisitorConfigurerByType(v1.VisitorType(typ))
		// 	if vc == nil {
		// 		panic("visitor type: " + typ + " not support")
		// 	}
		// 	visitorCmd := NewVisitorSubCommand(string(typ), vc, &clientCfg)
		// 	config.RegisterVisitorFlags(visitorCmd, vc)
		// 	cmd.AddCommand(visitorCmd)
		// }
		rootCmd.AddCommand(cmd)
	}
}

func NewForwardCommand(name string, c v1.ForwardConfigurer, clientCfg *v1.ClientCommonConfig) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("Run monitoragentc with a single %s forward", name),
		Run: func(cmd *cobra.Command, args []string) {
			clientCfg.Complete()
			if _, err := validation.ValidateClientCommonConfig(clientCfg); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			c.Complete(clientCfg.User)
			c.GetBaseConfig().Type = name
			if err := validation.ValidateForwardConfigurerForClient(c); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			err := startForwardService(clientCfg, []v1.ForwardConfigurer{c})
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}
}

func startForwardService(
	cfg *v1.ClientCommonConfig,
	forwardCfgs []v1.ForwardConfigurer,
) error {
	log.InitLog(cfg.Log.To, cfg.Log.Level, cfg.Log.MaxDays, cfg.Log.DisablePrintColor)

	svc, err := client.NewService(client.ServiceOptions{
		Common:      cfg,
		ForwardCfgs: forwardCfgs,
	})
	if err != nil {
		return err
	}

	shouldGracefulClose := cfg.Transport.Protocol == "kcp" || cfg.Transport.Protocol == "quic"
	// Capture the exit signal if we use kcp or quic.
	if shouldGracefulClose {
		go handleTermSignal(svc)
	}

	return svc.Run(context.Background())
}
