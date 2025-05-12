// Copyright 2017 vpp_team, vpp_team@gmail.com
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

package forward

import (
	"context"
	"io"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	libio "github.com/fatedier/golib/io"
	libdial "github.com/fatedier/golib/net/dial"
	pp "github.com/pires/go-proxyproto"
	"golang.org/x/time/rate"

	"monitoragent/pkg/config/types"
	v1 "monitoragent/pkg/config/v1"
	"monitoragent/pkg/msg"
	plugin "monitoragent/pkg/plugin/client"
	"monitoragent/pkg/transport"
	"monitoragent/pkg/util/limit"
	"monitoragent/pkg/util/xlog"
)

var forwardFactoryRegistry = map[reflect.Type]func(*BaseForward, v1.ForwardConfigurer) Forward{}

func RegisterForwardFactory(
	forwardConfType reflect.Type,
	factory func(*BaseForward, v1.ForwardConfigurer) Forward,
) {
	forwardFactoryRegistry[forwardConfType] = factory
}

// Forward defines how to handle work connections for different forward type.
type Forward interface {
	Run() error
	// InWorkConn accept work connections registered to server.
	InWorkConn(net.Conn, *msg.StartWorkConn)
	SetInWorkConnCallback(
		func(*v1.ForwardBaseConfig, net.Conn, *msg.StartWorkConn) /* continue */ bool,
	)
	Close()
}

func NewForward(
	ctx context.Context,
	fwdConf v1.ForwardConfigurer,
	clientCfg *v1.ClientCommonConfig,
	msgTransporter transport.MessageTransporter,
) (fwd Forward) {
	var limiter *rate.Limiter
	limitBytes := fwdConf.GetBaseConfig().Transport.BandwidthLimit.Bytes()
	if limitBytes > 0 &&
		fwdConf.GetBaseConfig().Transport.BandwidthLimitMode == types.BandwidthLimitModeClient {
		limiter = rate.NewLimiter(rate.Limit(float64(limitBytes)), int(limitBytes))
	}

	baseForward := BaseForward{
		baseCfg:        fwdConf.GetBaseConfig(),
		clientCfg:      clientCfg,
		limiter:        limiter,
		msgTransporter: msgTransporter,
		xl:             xlog.FromContextSafe(ctx),
		ctx:            ctx,
	}

	factory := forwardFactoryRegistry[reflect.TypeOf(fwdConf)]
	if factory == nil {
		return nil
	}
	return factory(&baseForward, fwdConf)
}

type BaseForward struct {
	baseCfg        *v1.ForwardBaseConfig
	clientCfg      *v1.ClientCommonConfig
	msgTransporter transport.MessageTransporter
	limiter        *rate.Limiter
	// forwardPlugin is used to handle connections instead of dialing to local service.
	// It's only validate for TCP protocol now.
	forwardPlugin      plugin.Plugin
	inWorkConnCallback func(*v1.ForwardBaseConfig, net.Conn, *msg.StartWorkConn) /* continue */ bool

	mu  sync.RWMutex
	xl  *xlog.Logger
	ctx context.Context
}

func (fwd *BaseForward) Run() error {
	if fwd.baseCfg.Plugin.Type != "" {
		p, err := plugin.Create(fwd.baseCfg.Plugin.Type, fwd.baseCfg.Plugin.ClientPluginOptions)
		if err != nil {
			return err
		}
		fwd.forwardPlugin = p
	}
	return nil
}

func (fwd *BaseForward) Close() {
	if fwd.forwardPlugin != nil {
		fwd.forwardPlugin.Close()
	}
}

func (fwd *BaseForward) SetInWorkConnCallback(
	cb func(*v1.ForwardBaseConfig, net.Conn, *msg.StartWorkConn) bool,
) {
	fwd.inWorkConnCallback = cb
}

func (fwd *BaseForward) InWorkConn(conn net.Conn, m *msg.StartWorkConn) {
	if fwd.inWorkConnCallback != nil {
		if !fwd.inWorkConnCallback(fwd.baseCfg, conn, m) {
			return
		}
	}
	fwd.HandleTCPWorkConnection(conn, m, []byte(fwd.clientCfg.Auth.Token))
}

// Common handler for tcp work connections.
func (fwd *BaseForward) HandleTCPWorkConnection(
	workConn net.Conn,
	m *msg.StartWorkConn,
	encKey []byte,
) {
	xl := fwd.xl
	baseCfg := fwd.baseCfg
	var (
		remote io.ReadWriteCloser
		err    error
	)
	remote = workConn
	if fwd.limiter != nil {
		remote = libio.WrapReadWriteCloser(
			limit.NewReader(workConn, fwd.limiter),
			limit.NewWriter(workConn, fwd.limiter),
			func() error {
				return workConn.Close()
			},
		)
	}

	xl.Trace("handle tcp work connection, useEncryption: %t, useCompression: %t",
		baseCfg.Transport.UseEncryption, baseCfg.Transport.UseCompression)
	if baseCfg.Transport.UseEncryption {
		remote, err = libio.WithEncryption(remote, encKey)
		if err != nil {
			workConn.Close()
			xl.Error("create encryption stream error: %v", err)
			return
		}
	}
	var compressionResourceRecycleFn func()
	if baseCfg.Transport.UseCompression {
		remote, compressionResourceRecycleFn = libio.WithCompressionFromPool(remote)
	}

	// check if we need to send forward protocol info
	var extraInfo plugin.ExtraInfo
	if baseCfg.Transport.ForwardProtocolVersion != "" {
		if m.SrcAddr != "" && m.SrcPort != 0 {
			if m.DstAddr == "" {
				m.DstAddr = "127.0.0.1"
			}
			srcAddr, _ := net.ResolveTCPAddr(
				"tcp",
				net.JoinHostPort(m.SrcAddr, strconv.Itoa(int(m.SrcPort))),
			)
			dstAddr, _ := net.ResolveTCPAddr(
				"tcp",
				net.JoinHostPort(m.DstAddr, strconv.Itoa(int(m.DstPort))),
			)
			h := &pp.Header{
				Command:         pp.PROXY,
				SourceAddr:      srcAddr,
				DestinationAddr: dstAddr,
			}

			if strings.Contains(m.SrcAddr, ".") {
				h.TransportProtocol = pp.TCPv4
			} else {
				h.TransportProtocol = pp.TCPv6
			}

			if baseCfg.Transport.ForwardProtocolVersion == "v1" {
				h.Version = 1
			} else if baseCfg.Transport.ForwardProtocolVersion == "v2" {
				h.Version = 2
			}

			extraInfo.ForwardProtocolHeader = h
		}
	}

	if fwd.forwardPlugin != nil {
		// if plugin is set, let plugin handle connection first
		xl.Debug("handle by plugin: %s", fwd.forwardPlugin.Name())
		fwd.forwardPlugin.Handle(remote, workConn, &extraInfo)
		xl.Debug("handle by plugin finished")
		return
	}

	localConn, err := libdial.Dial(
		net.JoinHostPort(baseCfg.LocalIP, strconv.Itoa(baseCfg.LocalPort)),
		libdial.WithTimeout(10*time.Second),
	)
	if err != nil {
		workConn.Close()
		xl.Error(
			"connect to local service [%s:%d] error: %v",
			baseCfg.LocalIP,
			baseCfg.LocalPort,
			err,
		)
		return
	}

	xl.Debug(
		"join connections, localConn(l[%s] r[%s]) workConn(l[%s] r[%s])",
		localConn.LocalAddr().String(),
		localConn.RemoteAddr().
			String(),
		workConn.LocalAddr().String(),
		workConn.RemoteAddr().String(),
	)

	if extraInfo.ForwardProtocolHeader != nil {
		if _, err := extraInfo.ForwardProtocolHeader.WriteTo(localConn); err != nil {
			workConn.Close()
			xl.Error("write forward protocol header to local conn error: %v", err)
			return
		}
	}

	// _, _, errs := libio.Join(localConn, remote)
	// xl.Debug("join connections closed")
	// if len(errs) > 0 {
	// 	xl.Trace("join connections errors: %v", errs)
	// }
	if compressionResourceRecycleFn != nil {
		compressionResourceRecycleFn()
	}
}
