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

//go:build !monitoragents

package forward

import (
	"io"
	"net"
	"reflect"
	"time"

	fmux "github.com/hashicorp/yamux"
	"github.com/quic-go/quic-go"

	v1 "monitoragent/pkg/config/v1"
	"monitoragent/pkg/msg"
	"monitoragent/pkg/nathole"
	"monitoragent/pkg/transport"
	netpkg "monitoragent/pkg/util/net"
)

func init() {
	RegisterForwardFactory(reflect.TypeOf(&v1.XTCPForwardConfig{}), NewXTCPForward)
}

type XTCPForward struct {
	*BaseForward

	cfg *v1.XTCPForwardConfig
}

func NewXTCPForward(baseForward *BaseForward, cfg v1.ForwardConfigurer) Forward {
	unwrapped, ok := cfg.(*v1.XTCPForwardConfig)
	if !ok {
		return nil
	}
	return &XTCPForward{
		BaseForward: baseForward,
		cfg:         unwrapped,
	}
}

func (fwd *XTCPForward) InWorkConn(conn net.Conn, startWorkConnMsg *msg.StartWorkConn) {
	xl := fwd.xl
	defer conn.Close()
	var natHoleSidMsg msg.NatHoleSid
	err := msg.ReadMsgInto(conn, &natHoleSidMsg)
	if err != nil {
		xl.Error("xtcp read from workConn error: %v", err)
		return
	}

	xl.Trace("nathole prepare start")
	prepareResult, err := nathole.Prepare([]string{fwd.clientCfg.NatHoleSTUNServer})
	if err != nil {
		xl.Warn("nathole prepare error: %v", err)
		return
	}
	xl.Info(
		"nathole prepare success, nat type: %s, behavior: %s, addresses: %v, assistedAddresses: %v",
		prepareResult.NatType,
		prepareResult.Behavior,
		prepareResult.Addrs,
		prepareResult.AssistedAddrs,
	)
	defer prepareResult.ListenConn.Close()

	// send NatHoleClient msg to server
	transactionID := nathole.NewTransactionID()
	natHoleClientMsg := &msg.NatHoleClient{
		TransactionID: transactionID,
		ProxyName:     fwd.cfg.Name,
		Sid:           natHoleSidMsg.Sid,
		MappedAddrs:   prepareResult.Addrs,
		AssistedAddrs: prepareResult.AssistedAddrs,
	}

	xl.Trace("nathole exchange info start")
	natHoleRespMsg, err := nathole.ExchangeInfo(
		fwd.ctx,
		fwd.msgTransporter,
		transactionID,
		natHoleClientMsg,
		5*time.Second,
	)
	if err != nil {
		xl.Warn("nathole exchange info error: %v", err)
		return
	}

	xl.Info(
		"get natHoleRespMsg, sid [%s], protocol [%s], candidate address %v, assisted address %v, detectBehavior: %+v",
		natHoleRespMsg.Sid,
		natHoleRespMsg.Protocol,
		natHoleRespMsg.CandidateAddrs,
		natHoleRespMsg.AssistedAddrs,
		natHoleRespMsg.DetectBehavior,
	)

	listenConn := prepareResult.ListenConn
	newListenConn, raddr, err := nathole.MakeHole(
		fwd.ctx,
		listenConn,
		natHoleRespMsg,
		[]byte(fwd.cfg.Secretkey),
	)
	if err != nil {
		listenConn.Close()
		xl.Warn("make hole error: %v", err)
		_ = fwd.msgTransporter.Send(&msg.NatHoleReport{
			Sid:     natHoleRespMsg.Sid,
			Success: false,
		})
		return
	}
	listenConn = newListenConn
	xl.Info(
		"establishing nat hole connection successful, sid [%s], remoteAddr [%s]",
		natHoleRespMsg.Sid,
		raddr,
	)

	_ = fwd.msgTransporter.Send(&msg.NatHoleReport{
		Sid:     natHoleRespMsg.Sid,
		Success: true,
	})

	if natHoleRespMsg.Protocol == "kcp" {
		fwd.listenByKCP(listenConn, raddr, startWorkConnMsg)
		return
	}

	// default is quic
	fwd.listenByQUIC(listenConn, raddr, startWorkConnMsg)
}

func (fwd *XTCPForward) listenByKCP(
	listenConn *net.UDPConn,
	raddr *net.UDPAddr,
	startWorkConnMsg *msg.StartWorkConn,
) {
	xl := fwd.xl
	listenConn.Close()
	laddr, _ := net.ResolveUDPAddr("udp", listenConn.LocalAddr().String())
	lConn, err := net.DialUDP("udp", laddr, raddr)
	if err != nil {
		xl.Warn("dial udp error: %v", err)
		return
	}
	defer lConn.Close()

	remote, err := netpkg.NewKCPConnFromUDP(lConn, true, raddr.String())
	if err != nil {
		xl.Warn("create kcp connection from udp connection error: %v", err)
		return
	}

	fmuxCfg := fmux.DefaultConfig()
	fmuxCfg.KeepAliveInterval = 10 * time.Second
	fmuxCfg.MaxStreamWindowSize = 6 * 1024 * 1024
	fmuxCfg.LogOutput = io.Discard
	session, err := fmux.Server(remote, fmuxCfg)
	if err != nil {
		xl.Error("create mux session error: %v", err)
		return
	}
	defer session.Close()

	for {
		muxConn, err := session.Accept()
		if err != nil {
			xl.Error("accept connection error: %v", err)
			return
		}
		go fwd.HandleTCPWorkConnection(muxConn, startWorkConnMsg, []byte(fwd.cfg.Secretkey))
	}
}

func (fwd *XTCPForward) listenByQUIC(
	listenConn *net.UDPConn,
	_ *net.UDPAddr,
	startWorkConnMsg *msg.StartWorkConn,
) {
	xl := fwd.xl
	defer listenConn.Close()

	tlsConfig, err := transport.NewServerTLSConfig("", "", "")
	if err != nil {
		xl.Warn("create tls config error: %v", err)
		return
	}
	tlsConfig.NextProtos = []string{"monitoragent"}
	quicListener, err := quic.Listen(listenConn, tlsConfig,
		&quic.Config{
			MaxIdleTimeout: time.Duration(
				fwd.clientCfg.Transport.QUIC.MaxIdleTimeout,
			) * time.Second,
			MaxIncomingStreams: int64(fwd.clientCfg.Transport.QUIC.MaxIncomingStreams),
			KeepAlivePeriod: time.Duration(
				fwd.clientCfg.Transport.QUIC.KeepalivePeriod,
			) * time.Second,
		},
	)
	if err != nil {
		xl.Warn("dial quic error: %v", err)
		return
	}
	// only accept one connection from raddr
	c, err := quicListener.Accept(fwd.ctx)
	if err != nil {
		xl.Error("quic accept connection error: %v", err)
		return
	}
	for {
		stream, err := c.AcceptStream(fwd.ctx)
		if err != nil {
			xl.Debug("quic accept stream error: %v", err)
			_ = c.CloseWithError(0, "")
			return
		}
		go fwd.HandleTCPWorkConnection(
			netpkg.QuicStreamToNetConn(stream, c),
			startWorkConnMsg,
			[]byte(fwd.cfg.Secretkey),
		)
	}
}
