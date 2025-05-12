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
	"strconv"
	"time"

	"github.com/fatedier/golib/errors"
	libio "github.com/fatedier/golib/io"

	v1 "monitoragent/pkg/config/v1"
	"monitoragent/pkg/msg"
	"monitoragent/pkg/proto/udp"
	"monitoragent/pkg/util/limit"
	netpkg "monitoragent/pkg/util/net"
)

func init() {
	RegisterForwardFactory(reflect.TypeOf(&v1.UDPForwardConfig{}), NewUDPForward)
}

type UDPForward struct {
	*BaseForward

	cfg *v1.UDPForwardConfig

	localAddr *net.UDPAddr
	readCh    chan *msg.UDPPacket

	// include msg.UDPPacket and msg.Ping
	sendCh   chan msg.Message
	workConn net.Conn
	closed   bool
}

func NewUDPForward(baseForward *BaseForward, cfg v1.ForwardConfigurer) Forward {
	unwrapped, ok := cfg.(*v1.UDPForwardConfig)
	if !ok {
		return nil
	}
	return &UDPForward{
		BaseForward: baseForward,
		cfg:         unwrapped,
	}
}

func (fwd *UDPForward) Run() (err error) {
	fwd.localAddr, err = net.ResolveUDPAddr(
		"udp",
		net.JoinHostPort(fwd.cfg.LocalIP, strconv.Itoa(fwd.cfg.LocalPort)),
	)
	if err != nil {
		return
	}
	return
}

func (fwd *UDPForward) Close() {
	fwd.mu.Lock()
	defer fwd.mu.Unlock()

	if !fwd.closed {
		fwd.closed = true
		if fwd.workConn != nil {
			fwd.workConn.Close()
		}
		if fwd.readCh != nil {
			close(fwd.readCh)
		}
		if fwd.sendCh != nil {
			close(fwd.sendCh)
		}
	}
}

func (fwd *UDPForward) InWorkConn(conn net.Conn, _ *msg.StartWorkConn) {
	xl := fwd.xl
	xl.Info("incoming a new work connection for udp forward, %s", conn.RemoteAddr().String())
	// close resources related with old workConn
	fwd.Close()

	var rwc io.ReadWriteCloser = conn
	var err error
	if fwd.limiter != nil {
		rwc = libio.WrapReadWriteCloser(
			limit.NewReader(conn, fwd.limiter),
			limit.NewWriter(conn, fwd.limiter),
			func() error {
				return conn.Close()
			},
		)
	}
	if fwd.cfg.Transport.UseEncryption {
		rwc, err = libio.WithEncryption(rwc, []byte(fwd.clientCfg.Auth.Token))
		if err != nil {
			conn.Close()
			xl.Error("create encryption stream error: %v", err)
			return
		}
	}
	if fwd.cfg.Transport.UseCompression {
		rwc = libio.WithCompression(rwc)
	}
	conn = netpkg.WrapReadWriteCloserToConn(rwc, conn)

	fwd.mu.Lock()
	fwd.workConn = conn
	fwd.readCh = make(chan *msg.UDPPacket, 1024)
	fwd.sendCh = make(chan msg.Message, 1024)
	fwd.closed = false
	fwd.mu.Unlock()

	workConnReaderFn := func(conn net.Conn, readCh chan *msg.UDPPacket) {
		for {
			var udpMsg msg.UDPPacket
			if errRet := msg.ReadMsgInto(conn, &udpMsg); errRet != nil {
				xl.Warn("read from workConn for udp error: %v", errRet)
				return
			}
			if errRet := errors.PanicToError(func() {
				xl.Trace("get udp package from workConn: %s", udpMsg.Content)
				readCh <- &udpMsg
			}); errRet != nil {
				xl.Info("reader goroutine for udp work connection closed: %v", errRet)
				return
			}
		}
	}
	workConnSenderFn := func(conn net.Conn, sendCh chan msg.Message) {
		defer func() {
			xl.Info("writer goroutine for udp work connection closed")
		}()
		var errRet error
		for rawMsg := range sendCh {
			switch m := rawMsg.(type) {
			case *msg.UDPPacket:
				xl.Trace("send udp package to workConn: %s", m.Content)
			case *msg.Ping:
				xl.Trace("send ping message to udp workConn")
			}
			if errRet = msg.WriteMsg(conn, rawMsg); errRet != nil {
				xl.Error("udp work write error: %v", errRet)
				return
			}
		}
	}
	heartbeatFn := func(sendCh chan msg.Message) {
		var errRet error
		for {
			time.Sleep(time.Duration(30) * time.Second)
			if errRet = errors.PanicToError(func() {
				sendCh <- &msg.Ping{}
			}); errRet != nil {
				xl.Trace("heartbeat goroutine for udp work connection closed")
				break
			}
		}
	}

	go workConnSenderFn(fwd.workConn, fwd.sendCh)
	go workConnReaderFn(fwd.workConn, fwd.readCh)
	go heartbeatFn(fwd.sendCh)
	udp.Forwarder(fwd.localAddr, fwd.readCh, fwd.sendCh, int(fwd.clientCfg.UDPPacketSize))
}
