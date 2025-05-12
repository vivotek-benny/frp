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

package v1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/samber/lo"

	"monitoragent/pkg/config/types"
	"monitoragent/pkg/msg"
	"monitoragent/pkg/util/util"
)

type ProxyTransport struct {
	// UseEncryption controls whether or not communication with the server will
	// be encrypted. Encryption is done using the tokens supplied in the server
	// and client configuration.
	UseEncryption bool `json:"useEncryption,omitempty"`
	// UseCompression controls whether or not communication with the server
	// will be compressed.
	UseCompression bool `json:"useCompression,omitempty"`
	// BandwidthLimit limit the bandwidth
	// 0 means no limit
	BandwidthLimit types.BandwidthQuantity `json:"bandwidthLimit,omitempty"`
	// BandwidthLimitMode specifies whether to limit the bandwidth on the
	// client or server side. Valid values include "client" and "server".
	// By default, this value is "client".
	BandwidthLimitMode string `json:"bandwidthLimitMode,omitempty"`
	// ForwardProtocolVersion specifies which protocol version to use. Valid
	// values include "v1", "v2", and "". If the value is "", a protocol
	// version will be automatically selected. By default, this value is "".
	ForwardProtocolVersion string `json:"forwardProtocolVersion,omitempty"`
}

type LoadBalancerConfig struct {
	// Group specifies which group the is a part of. The server will use
	// this information to load balance proxies in the same group. If the value
	// is "", this will not be in a group.
	Group string `json:"group"`
	// GroupKey specifies a group key, which should be the same among proxies
	// of the same group.
	GroupKey string `json:"groupKey,omitempty"`
}

type ProxyBackend struct {
	// LocalIP specifies the IP address or host name of the backend.
	LocalIP string `json:"localIP,omitempty"`
	// LocalPort specifies the port of the backend.
	LocalPort int `json:"localPort,omitempty"`

	// Plugin specifies what plugin should be used for handling connections. If this value
	// is set, the LocalIP and LocalPort values will be ignored.
	Plugin TypedClientPluginOptions `json:"plugin,omitempty"`
}

// HealthCheckConfig configures health checking. This can be useful for load
// balancing purposes to detect and remove proxies to failing services.
type HealthCheckConfig struct {
	// Type specifies what protocol to use for health checking.
	// Valid values include "tcp", "http", and "". If this value is "", health
	// checking will not be performed.
	//
	// If the type is "tcp", a connection will be attempted to the target
	// server. If a connection cannot be established, the health check fails.
	//
	// If the type is "http", a GET request will be made to the endpoint
	// specified by HealthCheckURL. If the response is not a 200, the health
	// check fails.
	Type string `json:"type"` // tcp | http
	// TimeoutSeconds specifies the number of seconds to wait for a health
	// check attempt to connect. If the timeout is reached, this counts as a
	// health check failure. By default, this value is 3.
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
	// MaxFailed specifies the number of allowed failures before the
	// is stopped. By default, this value is 1.
	MaxFailed int `json:"maxFailed,omitempty"`
	// IntervalSeconds specifies the time in seconds between health
	// checks. By default, this value is 10.
	IntervalSeconds int `json:"intervalSeconds"`
	// Path specifies the path to send health checks to if the
	// health check type is "http".
	Path string `json:"path,omitempty"`
}

type DomainConfig struct {
	CustomDomains []string `json:"customDomains,omitempty"`
	SubDomain     string   `json:"subdomain,omitempty"`
}

type ForwardBaseConfig struct {
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Transport ProxyTransport `json:"transport,omitempty"`
	// metadata info for each proxy
	Metadatas    map[string]string  `json:"metadatas,omitempty"`
	LoadBalancer LoadBalancerConfig `json:"loadBalancer,omitempty"`
	HealthCheck  HealthCheckConfig  `json:"healthCheck,omitempty"`
	ProxyBackend
}

func (c *ForwardBaseConfig) GetBaseConfig() *ForwardBaseConfig {
	return c
}

func (c *ForwardBaseConfig) Complete(namePrefix string) {
	c.Name = lo.Ternary(namePrefix == "", "", namePrefix+".") + c.Name
	c.LocalIP = util.EmptyOr(c.LocalIP, "127.0.0.1")
	c.Transport.BandwidthLimitMode = util.EmptyOr(
		c.Transport.BandwidthLimitMode,
		types.BandwidthLimitModeClient,
	)
}

func (c *ForwardBaseConfig) MarshalToMsg(m *msg.NewForward) {
	m.ForwardName = c.Name
	m.ForwardType = c.Type
	m.UseEncryption = c.Transport.UseEncryption
	m.UseCompression = c.Transport.UseCompression
	m.BandwidthLimit = c.Transport.BandwidthLimit.String()
	// leave it empty for default value to reduce traffic
	if c.Transport.BandwidthLimitMode != "client" {
		m.BandwidthLimitMode = c.Transport.BandwidthLimitMode
	}
	m.Group = c.LoadBalancer.Group
	m.GroupKey = c.LoadBalancer.GroupKey
	m.Metas = c.Metadatas
}

func (c *ForwardBaseConfig) UnmarshalFromMsg(m *msg.NewForward) {
	c.Name = m.ForwardName
	c.Type = m.ForwardType
	c.Transport.UseEncryption = m.UseEncryption
	c.Transport.UseCompression = m.UseCompression
	if m.BandwidthLimit != "" {
		c.Transport.BandwidthLimit, _ = types.NewBandwidthQuantity(m.BandwidthLimit)
	}
	if m.BandwidthLimitMode != "" {
		c.Transport.BandwidthLimitMode = m.BandwidthLimitMode
	}
	c.LoadBalancer.Group = m.Group
	c.LoadBalancer.GroupKey = m.GroupKey
	c.Metadatas = m.Metas
}

type TypedProxyConfig struct {
	Type string `json:"type"`
	ForwardConfigurer
}

func (c *TypedProxyConfig) UnmarshalJSON(b []byte) error {
	if len(b) == 4 && string(b) == "null" {
		return errors.New("type is required")
	}

	typeStruct := struct {
		Type string `json:"type"`
	}{}
	if err := json.Unmarshal(b, &typeStruct); err != nil {
		return err
	}

	c.Type = typeStruct.Type
	configurer := NewForwardConfigurerByType(ForwardType(typeStruct.Type))
	if configurer == nil {
		return fmt.Errorf("unknown proxy type: %s", typeStruct.Type)
	}
	decoder := json.NewDecoder(bytes.NewBuffer(b))
	if DisallowUnknownFields {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(configurer); err != nil {
		return err
	}
	c.ForwardConfigurer = configurer
	return nil
}

type ForwardConfigurer interface {
	Complete(namePrefix string)
	GetBaseConfig() *ForwardBaseConfig
	// MarshalToMsg marshals this config into a msg.NewProxy message. This
	// function will be called on the monitoragentc side.
	MarshalToMsg(*msg.NewForward)
	// UnmarshalFromMsg unmarshals a msg.NewProxy message into this config.
	// This function will be called on the monitoragents side.
	UnmarshalFromMsg(*msg.NewForward)
}

type ForwardType string

const (
	ForwardTypeTCP    ForwardType = "tcp"
	ForwardTypeUDP    ForwardType = "udp"
	ForwardTypeTCPMUX ForwardType = "tcpmux"
	ForwardTypeHTTP   ForwardType = "http"
	ForwardTypeHTTPS  ForwardType = "https"
	ForwardTypeSTCP   ForwardType = "stcp"
	ForwardTypeXTCP   ForwardType = "xtcp"
	ForwardTypeSUDP   ForwardType = "sudp"
)

var proxyConfigTypeMap = map[ForwardType]reflect.Type{
	ForwardTypeTCP:    reflect.TypeOf(TCPForwardConfig{}),
	ForwardTypeUDP:    reflect.TypeOf(UDPForwardConfig{}),
	ForwardTypeHTTP:   reflect.TypeOf(HTTPForwardConfig{}),
	ForwardTypeHTTPS:  reflect.TypeOf(HTTPSForwardConfig{}),
	ForwardTypeTCPMUX: reflect.TypeOf(TCPMuxForwardConfig{}),
	ForwardTypeSTCP:   reflect.TypeOf(STCPForwardConfig{}),
	ForwardTypeXTCP:   reflect.TypeOf(XTCPForwardConfig{}),
	ForwardTypeSUDP:   reflect.TypeOf(SUDPForwardConfig{}),
}

func NewForwardConfigurerByType(proxyType ForwardType) ForwardConfigurer {
	v, ok := proxyConfigTypeMap[proxyType]
	if !ok {
		return nil
	}
	pc := reflect.New(v).Interface().(ForwardConfigurer)
	pc.GetBaseConfig().Type = string(proxyType)
	return pc
}

var _ ForwardConfigurer = &TCPForwardConfig{}

type TCPForwardConfig struct {
	ForwardBaseConfig

	RemotePort int `json:"remotePort,omitempty"`
}

func (c *TCPForwardConfig) MarshalToMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.MarshalToMsg(m)

	m.RemotePort = c.RemotePort
}

func (c *TCPForwardConfig) UnmarshalFromMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.UnmarshalFromMsg(m)

	c.RemotePort = m.RemotePort
}

var _ ForwardConfigurer = &UDPForwardConfig{}

type UDPForwardConfig struct {
	ForwardBaseConfig

	RemotePort int `json:"remotePort,omitempty"`
}

func (c *UDPForwardConfig) MarshalToMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.MarshalToMsg(m)

	m.RemotePort = c.RemotePort
}

func (c *UDPForwardConfig) UnmarshalFromMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.UnmarshalFromMsg(m)

	c.RemotePort = m.RemotePort
}

var _ ForwardConfigurer = &HTTPForwardConfig{}

type HTTPForwardConfig struct {
	ForwardBaseConfig
	DomainConfig

	Locations         []string         `json:"locations,omitempty"`
	HTTPUser          string           `json:"httpUser,omitempty"`
	HTTPPassword      string           `json:"httpPassword,omitempty"`
	HostHeaderRewrite string           `json:"hostHeaderRewrite,omitempty"`
	RequestHeaders    HeaderOperations `json:"requestHeaders,omitempty"`
	RouteByHTTPUser   string           `json:"routeByHTTPUser,omitempty"`
}

func (c *HTTPForwardConfig) MarshalToMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.MarshalToMsg(m)

	m.CustomDomains = c.CustomDomains
	m.SubDomain = c.SubDomain
	m.Locations = c.Locations
	m.HostHeaderRewrite = c.HostHeaderRewrite
	m.HTTPUser = c.HTTPUser
	m.HTTPPwd = c.HTTPPassword
	m.Headers = c.RequestHeaders.Set
	m.RouteByHTTPUser = c.RouteByHTTPUser
}

func (c *HTTPForwardConfig) UnmarshalFromMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.UnmarshalFromMsg(m)

	c.CustomDomains = m.CustomDomains
	c.SubDomain = m.SubDomain
	c.Locations = m.Locations
	c.HostHeaderRewrite = m.HostHeaderRewrite
	c.HTTPUser = m.HTTPUser
	c.HTTPPassword = m.HTTPPwd
	c.RequestHeaders.Set = m.Headers
	c.RouteByHTTPUser = m.RouteByHTTPUser
}

var _ ForwardConfigurer = &HTTPSForwardConfig{}

type HTTPSForwardConfig struct {
	ForwardBaseConfig
	DomainConfig
}

func (c *HTTPSForwardConfig) MarshalToMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.MarshalToMsg(m)

	m.CustomDomains = c.CustomDomains
	m.SubDomain = c.SubDomain
}

func (c *HTTPSForwardConfig) UnmarshalFromMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.UnmarshalFromMsg(m)

	c.CustomDomains = m.CustomDomains
	c.SubDomain = m.SubDomain
}

type TCPMultiplexerType string

const (
	TCPMultiplexerHTTPConnect TCPMultiplexerType = "httpconnect"
)

var _ ForwardConfigurer = &TCPMuxForwardConfig{}

type TCPMuxForwardConfig struct {
	ForwardBaseConfig
	DomainConfig

	HTTPUser        string `json:"httpUser,omitempty"`
	HTTPPassword    string `json:"httpPassword,omitempty"`
	RouteByHTTPUser string `json:"routeByHTTPUser,omitempty"`
	Multiplexer     string `json:"multiplexer,omitempty"`
}

func (c *TCPMuxForwardConfig) MarshalToMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.MarshalToMsg(m)

	m.CustomDomains = c.CustomDomains
	m.SubDomain = c.SubDomain
	m.Multiplexer = c.Multiplexer
	m.HTTPUser = c.HTTPUser
	m.HTTPPwd = c.HTTPPassword
	m.RouteByHTTPUser = c.RouteByHTTPUser
}

func (c *TCPMuxForwardConfig) UnmarshalFromMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.UnmarshalFromMsg(m)

	c.CustomDomains = m.CustomDomains
	c.SubDomain = m.SubDomain
	c.Multiplexer = m.Multiplexer
	c.HTTPUser = m.HTTPUser
	c.HTTPPassword = m.HTTPPwd
	c.RouteByHTTPUser = m.RouteByHTTPUser
}

var _ ForwardConfigurer = &STCPForwardConfig{}

type STCPForwardConfig struct {
	ForwardBaseConfig

	Secretkey  string   `json:"secretKey,omitempty"`
	AllowUsers []string `json:"allowUsers,omitempty"`
}

func (c *STCPForwardConfig) MarshalToMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.MarshalToMsg(m)

	m.Sk = c.Secretkey
	m.AllowUsers = c.AllowUsers
}

func (c *STCPForwardConfig) UnmarshalFromMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.UnmarshalFromMsg(m)

	c.Secretkey = m.Sk
	c.AllowUsers = m.AllowUsers
}

var _ ForwardConfigurer = &XTCPForwardConfig{}

type XTCPForwardConfig struct {
	ForwardBaseConfig

	Secretkey  string   `json:"secretKey,omitempty"`
	AllowUsers []string `json:"allowUsers,omitempty"`
}

func (c *XTCPForwardConfig) MarshalToMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.MarshalToMsg(m)

	m.Sk = c.Secretkey
	m.AllowUsers = c.AllowUsers
}

func (c *XTCPForwardConfig) UnmarshalFromMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.UnmarshalFromMsg(m)

	c.Secretkey = m.Sk
	c.AllowUsers = m.AllowUsers
}

var _ ForwardConfigurer = &SUDPForwardConfig{}

type SUDPForwardConfig struct {
	ForwardBaseConfig

	Secretkey  string   `json:"secretKey,omitempty"`
	AllowUsers []string `json:"allowUsers,omitempty"`
}

func (c *SUDPForwardConfig) MarshalToMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.MarshalToMsg(m)

	m.Sk = c.Secretkey
	m.AllowUsers = c.AllowUsers
}

func (c *SUDPForwardConfig) UnmarshalFromMsg(m *msg.NewForward) {
	c.ForwardBaseConfig.UnmarshalFromMsg(m)

	c.Secretkey = m.Sk
	c.AllowUsers = m.AllowUsers
}
