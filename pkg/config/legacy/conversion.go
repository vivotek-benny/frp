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

package legacy

import (
	"strings"

	"github.com/samber/lo"

	"monitoragent/pkg/config/types"
	v1 "monitoragent/pkg/config/v1"
)

func Convert_ClientCommonConf_To_v1(conf *ClientCommonConf) *v1.ClientCommonConfig {
	out := &v1.ClientCommonConfig{}
	out.User = conf.User
	out.Auth.Method = v1.AuthMethod(conf.ClientConfig.AuthenticationMethod)
	out.Auth.Token = conf.ClientConfig.Token
	if conf.ClientConfig.AuthenticateHeartBeats {
		out.Auth.AdditionalScopes = append(out.Auth.AdditionalScopes, v1.AuthScopeHeartBeats)
	}
	if conf.ClientConfig.AuthenticateNewWorkConns {
		out.Auth.AdditionalScopes = append(out.Auth.AdditionalScopes, v1.AuthScopeNewWorkConns)
	}
	out.Auth.OIDC.ClientID = conf.ClientConfig.OidcClientID
	out.Auth.OIDC.ClientSecret = conf.ClientConfig.OidcClientSecret
	out.Auth.OIDC.Audience = conf.ClientConfig.OidcAudience
	out.Auth.OIDC.Scope = conf.ClientConfig.OidcScope
	out.Auth.OIDC.TokenEndpointURL = conf.ClientConfig.OidcTokenEndpointURL
	out.Auth.OIDC.AdditionalEndpointParams = conf.ClientConfig.OidcAdditionalEndpointParams

	out.ServerAddr = conf.ServerAddr
	out.ServerPort = conf.ServerPort
	out.NatHoleSTUNServer = conf.NatHoleSTUNServer
	out.Transport.DialServerTimeout = conf.DialServerTimeout
	out.Transport.DialServerKeepAlive = conf.DialServerKeepAlive
	out.Transport.ConnectServerLocalIP = conf.ConnectServerLocalIP
	out.Transport.ProxyURL = conf.HTTPProxy
	out.Transport.PoolCount = conf.PoolCount
	out.Transport.TCPMux = lo.ToPtr(conf.TCPMux)
	out.Transport.TCPMuxKeepaliveInterval = conf.TCPMuxKeepaliveInterval
	out.Transport.Protocol = conf.Protocol
	out.Transport.HeartbeatInterval = conf.HeartbeatInterval
	out.Transport.HeartbeatTimeout = conf.HeartbeatTimeout
	out.Transport.QUIC.KeepalivePeriod = conf.QUICKeepalivePeriod
	out.Transport.QUIC.MaxIdleTimeout = conf.QUICMaxIdleTimeout
	out.Transport.QUIC.MaxIncomingStreams = conf.QUICMaxIncomingStreams
	out.Transport.TLS.Enable = lo.ToPtr(conf.TLSEnable)
	out.Transport.TLS.DisableCustomTLSFirstByte = lo.ToPtr(conf.DisableCustomTLSFirstByte)
	out.Transport.TLS.TLSConfig.CertFile = conf.TLSCertFile
	out.Transport.TLS.TLSConfig.KeyFile = conf.TLSKeyFile
	out.Transport.TLS.TLSConfig.TrustedCaFile = conf.TLSTrustedCaFile
	out.Transport.TLS.TLSConfig.ServerName = conf.TLSServerName

	out.Log.To = conf.LogFile
	out.Log.Level = conf.LogLevel
	out.Log.MaxDays = conf.LogMaxDays
	out.Log.DisablePrintColor = conf.DisableLogColor

	out.WebServer.Addr = conf.AdminAddr
	out.WebServer.Port = conf.AdminPort
	out.WebServer.User = conf.AdminUser
	out.WebServer.Password = conf.AdminPwd
	out.WebServer.AssetsDir = conf.AssetsDir
	out.WebServer.PprofEnable = conf.PprofEnable

	out.DNSServer = conf.DNSServer
	out.LoginFailExit = lo.ToPtr(conf.LoginFailExit)
	out.Start = conf.Start
	out.UDPPacketSize = conf.UDPPacketSize
	out.Metadatas = conf.Metas
	out.IncludeConfigFiles = conf.IncludeConfigFiles
	return out
}

func Convert_ServerCommonConf_To_v1(conf *ServerCommonConf) *v1.ServerConfig {
	out := &v1.ServerConfig{}
	out.Auth.Method = v1.AuthMethod(conf.ServerConfig.AuthenticationMethod)
	out.Auth.Token = conf.ServerConfig.Token
	if conf.ServerConfig.AuthenticateHeartBeats {
		out.Auth.AdditionalScopes = append(out.Auth.AdditionalScopes, v1.AuthScopeHeartBeats)
	}
	if conf.ServerConfig.AuthenticateNewWorkConns {
		out.Auth.AdditionalScopes = append(out.Auth.AdditionalScopes, v1.AuthScopeNewWorkConns)
	}
	out.Auth.OIDC.Audience = conf.ServerConfig.OidcAudience
	out.Auth.OIDC.Issuer = conf.ServerConfig.OidcIssuer
	out.Auth.OIDC.SkipExpiryCheck = conf.ServerConfig.OidcSkipExpiryCheck
	out.Auth.OIDC.SkipIssuerCheck = conf.ServerConfig.OidcSkipIssuerCheck

	out.BindAddr = conf.BindAddr
	out.BindPort = conf.BindPort
	out.KCPBindPort = conf.KCPBindPort
	out.QUICBindPort = conf.QUICBindPort
	out.Transport.QUIC.KeepalivePeriod = conf.QUICKeepalivePeriod
	out.Transport.QUIC.MaxIdleTimeout = conf.QUICMaxIdleTimeout
	out.Transport.QUIC.MaxIncomingStreams = conf.QUICMaxIncomingStreams

	out.ProxyBindAddr = conf.ProxyBindAddr
	out.VhostHTTPPort = conf.VhostHTTPPort
	out.VhostHTTPSPort = conf.VhostHTTPSPort
	out.TCPMuxHTTPConnectPort = conf.TCPMuxHTTPConnectPort
	out.TCPMuxPassthrough = conf.TCPMuxPassthrough
	out.VhostHTTPTimeout = conf.VhostHTTPTimeout

	out.WebServer.Addr = conf.DashboardAddr
	out.WebServer.Port = conf.DashboardPort
	out.WebServer.User = conf.DashboardUser
	out.WebServer.Password = conf.DashboardPwd
	out.WebServer.AssetsDir = conf.AssetsDir
	if conf.DashboardTLSMode {
		out.WebServer.TLS = &v1.TLSConfig{}
		out.WebServer.TLS.CertFile = conf.DashboardTLSCertFile
		out.WebServer.TLS.KeyFile = conf.DashboardTLSKeyFile
		out.WebServer.PprofEnable = conf.PprofEnable
	}

	out.EnablePrometheus = conf.EnablePrometheus

	out.Log.To = conf.LogFile
	out.Log.Level = conf.LogLevel
	out.Log.MaxDays = conf.LogMaxDays
	out.Log.DisablePrintColor = conf.DisableLogColor

	out.DetailedErrorsToClient = lo.ToPtr(conf.DetailedErrorsToClient)
	out.SubDomainHost = conf.SubDomainHost
	out.Custom404Page = conf.Custom404Page
	out.UserConnTimeout = conf.UserConnTimeout
	out.UDPPacketSize = conf.UDPPacketSize
	out.NatHoleAnalysisDataReserveHours = conf.NatHoleAnalysisDataReserveHours

	out.Transport.TCPMux = lo.ToPtr(conf.TCPMux)
	out.Transport.TCPMuxKeepaliveInterval = conf.TCPMuxKeepaliveInterval
	out.Transport.TCPKeepAlive = conf.TCPKeepAlive
	out.Transport.MaxPoolCount = conf.MaxPoolCount
	out.Transport.HeartbeatTimeout = conf.HeartbeatTimeout

	out.Transport.TLS.Force = conf.TLSOnly
	out.Transport.TLS.CertFile = conf.TLSCertFile
	out.Transport.TLS.KeyFile = conf.TLSKeyFile
	out.Transport.TLS.TrustedCaFile = conf.TLSTrustedCaFile

	out.MaxPortsPerClient = conf.MaxPortsPerClient

	for _, v := range conf.HTTPPlugins {
		out.HTTPPlugins = append(out.HTTPPlugins, v1.HTTPPluginOptions{
			Name:      v.Name,
			Addr:      v.Addr,
			Path:      v.Path,
			Ops:       v.Ops,
			TLSVerify: v.TLSVerify,
		})
	}

	out.AllowPorts, _ = types.NewPortsRangeSliceFromString(conf.AllowPortsStr)
	return out
}

func transformHeadersFromPluginParams(params map[string]string) v1.HeaderOperations {
	out := v1.HeaderOperations{}
	for k, v := range params {
		if !strings.HasPrefix(k, "plugin_header_") {
			continue
		}
		if k = strings.TrimPrefix(k, "plugin_header_"); k != "" {
			out.Set[k] = v
		}
	}
	return out
}

func Convert_ForwardConf_To_v1_Base(conf ForwardConf) *v1.ForwardBaseConfig {
	out := &v1.ForwardBaseConfig{}
	base := conf.GetBaseConfig()

	out.Name = base.ForwardName
	out.Type = base.ForwardType
	out.Metadatas = base.Metas

	out.Transport.UseEncryption = base.UseEncryption
	out.Transport.UseCompression = base.UseCompression
	out.Transport.BandwidthLimit = base.BandwidthLimit
	out.Transport.BandwidthLimitMode = base.BandwidthLimitMode
	out.Transport.ForwardProtocolVersion = base.ProxyProtocolVersion

	out.LoadBalancer.Group = base.Group
	out.LoadBalancer.GroupKey = base.GroupKey

	out.HealthCheck.Type = base.HealthCheckType
	out.HealthCheck.TimeoutSeconds = base.HealthCheckTimeoutS
	out.HealthCheck.MaxFailed = base.HealthCheckMaxFailed
	out.HealthCheck.IntervalSeconds = base.HealthCheckIntervalS
	out.HealthCheck.Path = base.HealthCheckURL

	out.LocalIP = base.LocalIP
	out.LocalPort = base.LocalPort

	switch base.Plugin {
	case "http2https":
		out.Plugin.ClientPluginOptions = &v1.HTTP2HTTPSPluginOptions{
			LocalAddr:         base.PluginParams["plugin_local_addr"],
			HostHeaderRewrite: base.PluginParams["plugin_host_header_rewrite"],
			RequestHeaders:    transformHeadersFromPluginParams(base.PluginParams),
		}
	case "http_proxy":
		out.Plugin.ClientPluginOptions = &v1.HTTPForwardPluginOptions{
			HTTPUser:     base.PluginParams["plugin_http_user"],
			HTTPPassword: base.PluginParams["plugin_http_passwd"],
		}
	case "https2http":
		out.Plugin.ClientPluginOptions = &v1.HTTPS2HTTPPluginOptions{
			LocalAddr:         base.PluginParams["plugin_local_addr"],
			HostHeaderRewrite: base.PluginParams["plugin_host_header_rewrite"],
			RequestHeaders:    transformHeadersFromPluginParams(base.PluginParams),
			CrtPath:           base.PluginParams["plugin_crt_path"],
			KeyPath:           base.PluginParams["plugin_key_path"],
		}
	case "https2https":
		out.Plugin.ClientPluginOptions = &v1.HTTPS2HTTPSPluginOptions{
			LocalAddr:         base.PluginParams["plugin_local_addr"],
			HostHeaderRewrite: base.PluginParams["plugin_host_header_rewrite"],
			RequestHeaders:    transformHeadersFromPluginParams(base.PluginParams),
			CrtPath:           base.PluginParams["plugin_crt_path"],
			KeyPath:           base.PluginParams["plugin_key_path"],
		}
	case "socks5":
		out.Plugin.ClientPluginOptions = &v1.Socks5PluginOptions{
			Username: base.PluginParams["plugin_user"],
			Password: base.PluginParams["plugin_passwd"],
		}
	case "static_file":
		out.Plugin.ClientPluginOptions = &v1.StaticFilePluginOptions{
			LocalPath:    base.PluginParams["plugin_local_path"],
			StripPrefix:  base.PluginParams["plugin_strip_prefix"],
			HTTPUser:     base.PluginParams["plugin_http_user"],
			HTTPPassword: base.PluginParams["plugin_http_passwd"],
		}
	case "unix_domain_socket":
		out.Plugin.ClientPluginOptions = &v1.UnixDomainSocketPluginOptions{
			UnixPath: base.PluginParams["plugin_unix_path"],
		}
	}
	out.Plugin.Type = base.Plugin
	return out
}

func Convert_ForwardConf_To_v1(conf ForwardConf) v1.ForwardConfigurer {
	outBase := Convert_ForwardConf_To_v1_Base(conf)
	var out v1.ForwardConfigurer
	switch v := conf.(type) {
	case *TCPForwardConf:
		c := &v1.TCPForwardConfig{ForwardBaseConfig: *outBase}
		c.RemotePort = v.RemotePort
		out = c
	case *UDPForwardConf:
		c := &v1.UDPForwardConfig{ForwardBaseConfig: *outBase}
		c.RemotePort = v.RemotePort
		out = c
	case *HTTPForwardConf:
		c := &v1.HTTPForwardConfig{ForwardBaseConfig: *outBase}
		c.CustomDomains = v.CustomDomains
		c.SubDomain = v.SubDomain
		c.Locations = v.Locations
		c.HTTPUser = v.HTTPUser
		c.HTTPPassword = v.HTTPPwd
		c.HostHeaderRewrite = v.HostHeaderRewrite
		c.RequestHeaders.Set = v.Headers
		c.RouteByHTTPUser = v.RouteByHTTPUser
		out = c
	case *HTTPSForwardConf:
		c := &v1.HTTPSForwardConfig{ForwardBaseConfig: *outBase}
		c.CustomDomains = v.CustomDomains
		c.SubDomain = v.SubDomain
		out = c
	case *TCPMuxForwardConf:
		c := &v1.TCPMuxForwardConfig{ForwardBaseConfig: *outBase}
		c.CustomDomains = v.CustomDomains
		c.SubDomain = v.SubDomain
		c.HTTPUser = v.HTTPUser
		c.HTTPPassword = v.HTTPPwd
		c.RouteByHTTPUser = v.RouteByHTTPUser
		c.Multiplexer = v.Multiplexer
		out = c
	case *STCPForwardConf:
		c := &v1.STCPForwardConfig{ForwardBaseConfig: *outBase}
		c.Secretkey = v.Sk
		c.AllowUsers = v.AllowUsers
		out = c
	case *SUDPForwardConf:
		c := &v1.SUDPForwardConfig{ForwardBaseConfig: *outBase}
		c.Secretkey = v.Sk
		c.AllowUsers = v.AllowUsers
		out = c
	case *XTCPForwardConf:
		c := &v1.XTCPForwardConfig{ForwardBaseConfig: *outBase}
		c.Secretkey = v.Sk
		c.AllowUsers = v.AllowUsers
		out = c
	}
	return out
}

func Convert_VisitorConf_To_v1_Base(conf VisitorConf) *v1.VisitorBaseConfig {
	out := &v1.VisitorBaseConfig{}
	base := conf.GetBaseConfig()

	out.Name = base.ForwardName
	out.Type = base.ForwardType
	out.Transport.UseEncryption = base.UseEncryption
	out.Transport.UseCompression = base.UseCompression
	out.SecretKey = base.Sk
	out.ServerUser = base.ServerUser
	out.ServerName = base.ServerName
	out.BindAddr = base.BindAddr
	out.BindPort = base.BindPort
	return out
}

func Convert_VisitorConf_To_v1(conf VisitorConf) v1.VisitorConfigurer {
	outBase := Convert_VisitorConf_To_v1_Base(conf)
	var out v1.VisitorConfigurer
	switch v := conf.(type) {
	case *STCPVisitorConf:
		c := &v1.STCPVisitorConfig{VisitorBaseConfig: *outBase}
		out = c
	case *SUDPVisitorConf:
		c := &v1.SUDPVisitorConfig{VisitorBaseConfig: *outBase}
		out = c
	case *XTCPVisitorConf:
		c := &v1.XTCPVisitorConfig{VisitorBaseConfig: *outBase}
		c.Protocol = v.Protocol
		c.KeepTunnelOpen = v.KeepTunnelOpen
		c.MaxRetriesAnHour = v.MaxRetriesAnHour
		c.MinRetryInterval = v.MinRetryInterval
		c.FallbackTo = v.FallbackTo
		c.FallbackTimeoutMs = v.FallbackTimeoutMs
		out = c
	}
	return out
}
