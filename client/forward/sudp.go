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
	"sync"
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
	RegisterForwardFactory(reflect.TypeOf(&v1.SUDPForwardConfig{}), NewSUDPForward)
}

type SUDPForward struct {
	*BaseForward

	cfg *v1.SUDPForwardConfig

	localAddr *net.UDPAddr

	closeCh chan struct{}
}

func NewSUDPForward(baseForward *BaseForward, cfg v1.ForwardConfigurer) Forward {
	unwrapped, ok := cfg.(*v1.SUDPForwardConfig)
	if !ok {
		return nil
	}
	return &SUDPForward{
		BaseForward: baseForward,
		cfg:         unwrapped,
		closeCh:     make(chan struct{}),
	}
}

func (fwd *SUDPForward) Run() (err error) {
	fwd.localAddr, err = net.ResolveUDPAddr(
		"udp",
		net.JoinHostPort(fwd.cfg.LocalIP, strconv.Itoa(fwd.cfg.LocalPort)),
	)
	if err != nil {
		return
	}
	return
}

func (fwd *SUDPForward) Close() {
	fwd.mu.Lock()
	defer fwd.mu.Unlock()
	select {
	case <-fwd.closeCh:
		return
	default:
		close(fwd.closeCh)
	}
}

func (fwd *SUDPForward) InWorkConn(conn net.Conn, _ *msg.StartWorkConn) {
	xl := fwd.xl
	xl.Info("incoming a new work connection for sudp forward, %s", conn.RemoteAddr().String())

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

	workConn := conn
	readCh := make(chan *msg.UDPPacket, 1024)
	sendCh := make(chan msg.Message, 1024)
	isClose := false

	mu := &sync.Mutex{}

	closeFn := func() {
		mu.Lock()
		defer mu.Unlock()
		if isClose {
			return
		}

		isClose = true
		if workConn != nil {
			workConn.Close()
		}
		close(readCh)
		close(sendCh)
	}

	// udp service <- monitoragentc <- monitoragents <- monitoragentc visitor <- user
	workConnReaderFn := func(conn net.Conn, readCh chan *msg.UDPPacket) {
		defer closeFn()

		for {
			// first to check sudp forward is closed or not
			select {
			case <-fwd.closeCh:
				xl.Trace("monitoragentc sudp forward is closed")
				return
			default:
			}

			var udpMsg msg.UDPPacket
			if errRet := msg.ReadMsgInto(conn, &udpMsg); errRet != nil {
				xl.Warn("read from workConn for sudp error: %v", errRet)
				return
			}

			if errRet := errors.PanicToError(func() {
				readCh <- &udpMsg
			}); errRet != nil {
				xl.Warn("reader goroutine for sudp work connection closed: %v", errRet)
				return
			}
		}
	}

	// udp service -> monitoragentc -> monitoragents -> monitoragentc visitor -> user
	workConnSenderFn := func(conn net.Conn, sendCh chan msg.Message) {
		defer func() {
			closeFn()
			xl.Info("writer goroutine for sudp work connection closed")
		}()

		var errRet error
		for rawMsg := range sendCh {
			switch m := rawMsg.(type) {
			case *msg.UDPPacket:
				xl.Trace("monitoragentc send udp package to monitoragentc visitor, [udp local: %v, remote: %v], [tcp work conn local: %v, remote: %v]",
					m.LocalAddr.String(), m.RemoteAddr.String(), conn.LocalAddr().String(), conn.RemoteAddr().String())
			case *msg.Ping:
				xl.Trace("monitoragentc send ping message to monitoragentc visitor")
			}

			if errRet = msg.WriteMsg(conn, rawMsg); errRet != nil {
				xl.Error("sudp work write error: %v", errRet)
				return
			}
		}
	}

	heartbeatFn := func(sendCh chan msg.Message) {
		ticker := time.NewTicker(30 * time.Second)
		defer func() {
			ticker.Stop()
			closeFn()
		}()

		var errRet error
		for {
			select {
			case <-ticker.C:
				if errRet = errors.PanicToError(func() {
					sendCh <- &msg.Ping{}
				}); errRet != nil {
					xl.Warn("heartbeat goroutine for sudp work connection closed")
					return
				}
			case <-fwd.closeCh:
				xl.Trace("monitoragentc sudp forward is closed")
				return
			}
		}
	}

	go workConnSenderFn(workConn, sendCh)
	go workConnReaderFn(workConn, readCh)
	go heartbeatFn(sendCh)

	udp.Forwarder(fwd.localAddr, readCh, sendCh, int(fwd.clientCfg.UDPPacketSize))
}
