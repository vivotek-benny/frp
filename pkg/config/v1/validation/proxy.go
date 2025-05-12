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

package validation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/samber/lo"

	v1 "monitoragent/pkg/config/v1"
)

func validateForwardBaseConfigForClient(c *v1.ForwardBaseConfig) error {
	if c.Name == "" {
		return errors.New("name should not be empty")
	}

	if !lo.Contains([]string{"", "v1", "v2"}, c.Transport.ForwardProtocolVersion) {
		return fmt.Errorf(
			"not support forward protocol version: %s",
			c.Transport.ForwardProtocolVersion,
		)
	}

	if !lo.Contains([]string{"client", "server"}, c.Transport.BandwidthLimitMode) {
		return fmt.Errorf("bandwidth limit mode should be client or server")
	}

	if c.Plugin.Type == "" {
		if err := ValidatePort(c.LocalPort, "localPort"); err != nil {
			return fmt.Errorf("localPort: %v", err)
		}
	}

	if !lo.Contains([]string{"", "tcp", "http"}, c.HealthCheck.Type) {
		return fmt.Errorf("not support health check type: %s", c.HealthCheck.Type)
	}
	if c.HealthCheck.Type != "" {
		if c.HealthCheck.Type == "http" &&
			c.HealthCheck.Path == "" {
			return fmt.Errorf("health check path should not be empty")
		}
	}

	if c.Plugin.Type != "" {
		if err := ValidateClientPluginOptions(c.Plugin.ClientPluginOptions); err != nil {
			return fmt.Errorf("plugin %s: %v", c.Plugin.Type, err)
		}
	}
	return nil
}

func validateForwardBaseConfigForServer(c *v1.ForwardBaseConfig, s *v1.ServerConfig) error {
	return nil
}

func validateDomainConfigForClient(c *v1.DomainConfig) error {
	if c.SubDomain == "" && len(c.CustomDomains) == 0 {
		return errors.New("subdomain and custom domains should not be both empty")
	}
	return nil
}

func validateDomainConfigForServer(c *v1.DomainConfig, s *v1.ServerConfig) error {
	for _, domain := range c.CustomDomains {
		if s.SubDomainHost != "" &&
			len(strings.Split(s.SubDomainHost, ".")) < len(strings.Split(domain, ".")) {
			if strings.Contains(domain, s.SubDomainHost) {
				return fmt.Errorf(
					"custom domain [%s] should not belong to subdomain host [%s]",
					domain,
					s.SubDomainHost,
				)
			}
		}
	}

	if c.SubDomain != "" {
		if s.SubDomainHost == "" {
			return errors.New(
				"subdomain is not supported because this feature is not enabled in server",
			)
		}

		if strings.Contains(c.SubDomain, ".") || strings.Contains(c.SubDomain, "*") {
			return errors.New("'.' and '*' are not supported in subdomain")
		}
	}
	return nil
}

func ValidateForwardConfigurerForClient(c v1.ForwardConfigurer) error {
	base := c.GetBaseConfig()
	if err := validateForwardBaseConfigForClient(base); err != nil {
		return err
	}

	switch v := c.(type) {
	case *v1.TCPForwardConfig:
		return validateTCPForwardConfigForClient(v)
	case *v1.UDPForwardConfig:
		return validateUDPForwardConfigForClient(v)
	case *v1.TCPMuxForwardConfig:
		return validateTCPMuxForwardConfigForClient(v)
	case *v1.HTTPForwardConfig:
		return validateHTTPForwardConfigForClient(v)
	case *v1.HTTPSForwardConfig:
		return validateHTTPSForwardConfigForClient(v)
	case *v1.STCPForwardConfig:
		return validateSTCPForwardConfigForClient(v)
	case *v1.XTCPForwardConfig:
		return validateXTCPForwardConfigForClient(v)
	case *v1.SUDPForwardConfig:
		return validateSUDPForwardConfigForClient(v)
	}
	return errors.New("unknown forward config type")
}

func validateTCPForwardConfigForClient(c *v1.TCPForwardConfig) error {
	return nil
}

func validateUDPForwardConfigForClient(c *v1.UDPForwardConfig) error {
	return nil
}

func validateTCPMuxForwardConfigForClient(c *v1.TCPMuxForwardConfig) error {
	if err := validateDomainConfigForClient(&c.DomainConfig); err != nil {
		return err
	}

	if !lo.Contains([]string{string(v1.TCPMultiplexerHTTPConnect)}, c.Multiplexer) {
		return fmt.Errorf("not support multiplexer: %s", c.Multiplexer)
	}
	return nil
}

func validateHTTPForwardConfigForClient(c *v1.HTTPForwardConfig) error {
	return validateDomainConfigForClient(&c.DomainConfig)
}

func validateHTTPSForwardConfigForClient(c *v1.HTTPSForwardConfig) error {
	return validateDomainConfigForClient(&c.DomainConfig)
}

func validateSTCPForwardConfigForClient(c *v1.STCPForwardConfig) error {
	return nil
}

func validateXTCPForwardConfigForClient(c *v1.XTCPForwardConfig) error {
	return nil
}

func validateSUDPForwardConfigForClient(c *v1.SUDPForwardConfig) error {
	return nil
}

func ValidateForwardConfigurerForServer(c v1.ForwardConfigurer, s *v1.ServerConfig) error {
	base := c.GetBaseConfig()
	if err := validateForwardBaseConfigForServer(base, s); err != nil {
		return err
	}

	switch v := c.(type) {
	case *v1.TCPForwardConfig:
		return validateTCPForwardConfigForServer(v, s)
	case *v1.UDPForwardConfig:
		return validateUDPForwardConfigForServer(v, s)
	case *v1.TCPMuxForwardConfig:
		return validateTCPMuxForwardConfigForServer(v, s)
	case *v1.HTTPForwardConfig:
		return validateHTTPForwardConfigForServer(v, s)
	case *v1.HTTPSForwardConfig:
		return validateHTTPSForwardConfigForServer(v, s)
	case *v1.STCPForwardConfig:
		return validateSTCPForwardConfigForServer(v, s)
	case *v1.XTCPForwardConfig:
		return validateXTCPForwardConfigForServer(v, s)
	case *v1.SUDPForwardConfig:
		return validateSUDPForwardConfigForServer(v, s)
	default:
		return errors.New("unknown forward config type")
	}
}

func validateTCPForwardConfigForServer(c *v1.TCPForwardConfig, s *v1.ServerConfig) error {
	return nil
}

func validateUDPForwardConfigForServer(c *v1.UDPForwardConfig, s *v1.ServerConfig) error {
	return nil
}

func validateTCPMuxForwardConfigForServer(c *v1.TCPMuxForwardConfig, s *v1.ServerConfig) error {
	if c.Multiplexer == string(v1.TCPMultiplexerHTTPConnect) &&
		s.TCPMuxHTTPConnectPort == 0 {
		return fmt.Errorf(
			"tcpmux with multiplexer httpconnect not supported because this feature is not enabled in server",
		)
	}

	return validateDomainConfigForServer(&c.DomainConfig, s)
}

func validateHTTPForwardConfigForServer(c *v1.HTTPForwardConfig, s *v1.ServerConfig) error {
	if s.VhostHTTPPort == 0 {
		return fmt.Errorf("type [http] not supported when vhost http port is not set")
	}

	return validateDomainConfigForServer(&c.DomainConfig, s)
}

func validateHTTPSForwardConfigForServer(c *v1.HTTPSForwardConfig, s *v1.ServerConfig) error {
	if s.VhostHTTPSPort == 0 {
		return fmt.Errorf("type [https] not supported when vhost https port is not set")
	}

	return validateDomainConfigForServer(&c.DomainConfig, s)
}

func validateSTCPForwardConfigForServer(c *v1.STCPForwardConfig, s *v1.ServerConfig) error {
	return nil
}

func validateXTCPForwardConfigForServer(c *v1.XTCPForwardConfig, s *v1.ServerConfig) error {
	return nil
}

func validateSUDPForwardConfigForServer(c *v1.SUDPForwardConfig, s *v1.ServerConfig) error {
	return nil
}
