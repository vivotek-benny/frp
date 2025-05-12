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

package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/fatedier/golib/crypto"
	"github.com/samber/lo"

	"monitoragent/client/connector"
	"monitoragent/client/forward"
	"monitoragent/pkg/auth"
	v1 "monitoragent/pkg/config/v1"
	"monitoragent/pkg/msg"
	httppkg "monitoragent/pkg/util/http"
	"monitoragent/pkg/util/log"
	netpkg "monitoragent/pkg/util/net"
	"monitoragent/pkg/util/version"
	"monitoragent/pkg/util/wait"
	"monitoragent/pkg/util/xlog"
)

func init() {
	crypto.DefaultSalt = "frp"
}

type cancelErr struct {
	Err error
}

func (e cancelErr) Error() string {
	return e.Err.Error()
}

// ServiceOptions contains options for creating a new client service.
type ServiceOptions struct {
	Common      *v1.ClientCommonConfig
	ForwardCfgs []v1.ForwardConfigurer
	VisitorCfgs []v1.VisitorConfigurer

	// ConfigFilePath is the path to the configuration file used to initialize.
	// If it is empty, it means that the configuration file is not used for initialization.
	// It may be initialized using command line parameters or called directly.
	ConfigFilePath string

	// ClientSpec is the client specification that control the client behavior.
	ClientSpec *msg.ClientSpec

	// ConnectorCreator is a function that creates a new connector to make connections to the server.
	// The Connector shields the underlying connection details, whether it is through TCP or QUIC connection,
	// and regardless of whether multiplexing is used.
	//
	// If it is not set, the default monitoragentc connector will be used.
	// By using a custom Connector, it can be used to implement a VirtualClient, which connects to monitoragents
	// through a pipe instead of a real physical connection.
	ConnectorCreator func(context.Context, *v1.ClientCommonConfig) connector.Connector

	// HandleWorkConnCb is a callback function that is called when a new work connection is created.
	//
	// If it is not set, the default monitoragentc implementation will be used.
	HandleWorkConnCb func(*v1.ForwardBaseConfig, net.Conn, *msg.StartWorkConn) bool
}

// setServiceOptionsDefault sets the default values for ServiceOptions.
func setServiceOptionsDefault(options *ServiceOptions) {
	if options.Common != nil {
		options.Common.Complete()
	}
	if options.ConnectorCreator == nil {
		options.ConnectorCreator = connector.NewDefaultConnector
	}
}

// Service is the client service that connects to monitoragents and provides forward services.
type Service struct {
	ctlMu sync.RWMutex
	// manager control connection with server
	ctl *Control
	// Uniq id got from monitoragents, it will be attached to loginMsg.
	runID string

	// Sets authentication based on selected method
	authSetter auth.Setter

	// web server for admin UI and apis
	webServer *httppkg.Server

	cfgMu       sync.RWMutex
	common      *v1.ClientCommonConfig
	forwardCfgs []v1.ForwardConfigurer
	visitorCfgs []v1.VisitorConfigurer
	clientSpec  *msg.ClientSpec

	// The configuration file used to initialize this client, or an empty
	// string if no configuration file was used.
	configFilePath string

	// service context
	ctx context.Context
	// call cancel to stop service
	cancel                   context.CancelCauseFunc
	gracefulShutdownDuration time.Duration

	connectorCreator func(context.Context, *v1.ClientCommonConfig) connector.Connector
	handleWorkConnCb func(*v1.ForwardBaseConfig, net.Conn, *msg.StartWorkConn) bool
}

func NewService(options ServiceOptions) (*Service, error) {
	setServiceOptionsDefault(&options)

	ctx := context.Background()
	svc := &Service{
		ctx:              ctx,
		authSetter:       auth.NewAuthSetter(options.Common.Auth),
		common:           options.Common,
		configFilePath:   options.ConfigFilePath,
		forwardCfgs:      options.ForwardCfgs,
		visitorCfgs:      options.VisitorCfgs,
		clientSpec:       options.ClientSpec,
		connectorCreator: options.ConnectorCreator,
		handleWorkConnCb: options.HandleWorkConnCb,
	}

	return svc, nil
}

func (svc *Service) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancelCause(ctx)
	svc.ctx = xlog.NewContext(ctx, xlog.FromContextSafe(ctx))
	svc.cancel = cancel

	log.Info("monitoragent is starting...")

	if svc.common.DNSServer != "" {
		log.Info("Using custom DNS: %s", svc.common.DNSServer)
		netpkg.SetDefaultDNSAddress(svc.common.DNSServer)
	}

	svc.loopLoginUntilSuccess(10*time.Second, lo.FromPtr(svc.common.LoginFailExit))

	if svc.ctl == nil {
		cancelCause := cancelErr{}
		_ = errors.As(context.Cause(svc.ctx), &cancelCause)
		return fmt.Errorf(
			"login to the server failed: %v. This client will now exit.",
			cancelCause.Err,
		)
	}

	go svc.keepControllerWorking()

	log.Info("monitoragent is now running.")
	<-svc.ctx.Done()
	log.Info("monitoragent is shutting down.")
	svc.stop()
	return nil
}

func (svr *Service) keepControllerWorking() {
	<-svr.ctl.Done()

	// There is a situation where the login is successful but due to certain reasons,
	// the control immediately exits. It is necessary to limit the frequency of reconnection in this case.
	// The interval for the first three retries in 1 minute will be very short, and then it will increase exponentially.
	// The maximum interval is 20 seconds.
	wait.BackoffUntil(func() (bool, error) {
		// loopLoginUntilSuccess is another layer of loop that will continuously attempt to
		// login to the server until successful.
		svr.loopLoginUntilSuccess(20*time.Second, false)
		if svr.ctl != nil {
			<-svr.ctl.Done()
			return false, errors.New("control is closed and try another loop")
		}
		// If the control is nil, it means that the login failed and the service is also closed.
		return false, nil
	}, wait.NewFastBackoffManager(
		wait.FastBackoffOptions{
			Duration:        time.Second,
			Factor:          2,
			Jitter:          0.1,
			MaxDuration:     20 * time.Second,
			FastRetryCount:  3,
			FastRetryDelay:  200 * time.Millisecond,
			FastRetryWindow: time.Minute,
			FastRetryJitter: 0.5,
		},
	), true, svr.ctx.Done())
}

// login creates a connection to monitoragents and registers it self as a client
// conn: control connection
// session: if it's not nil, using tcp mux
func (svr *Service) login() (conn net.Conn, connector connector.Connector, err error) {
	xl := xlog.FromContextSafe(svr.ctx)
	connector = svr.connectorCreator(svr.ctx, svr.common)
	if err = connector.Open(); err != nil {
		return nil, nil, err
	}

	defer func() {
		if err != nil {
			connector.Close()
		}
	}()

	conn, err = connector.Connect()
	if err != nil {
		return
	}

	handshakeMsg := &msg.Handshake{
		Arch:      runtime.GOARCH,
		Os:        runtime.GOOS,
		PoolCount: svr.common.Transport.PoolCount,
		User:      svr.common.User,
		Version:   version.Full(),
		Timestamp: time.Now().Unix(),
		RunID:     svr.runID,
		Metas:     svr.common.Metadatas,
	}
	if svr.clientSpec != nil {
		handshakeMsg.ClientSpec = *svr.clientSpec
	}

	// Add auth
	if err = svr.authSetter.SetLogin(handshakeMsg); err != nil {
		return
	}

	if err = msg.WriteMsg(conn, handshakeMsg); err != nil {
		return
	}

	var handshakeAckMsg msg.HandshakeAck
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if err = msg.ReadMsgInto(conn, &handshakeAckMsg); err != nil {
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	if handshakeAckMsg.Error != "" {
		err = fmt.Errorf("%s", handshakeAckMsg.Error)
		xl.Error("%s", handshakeAckMsg.Error)
		return
	}

	svr.runID = handshakeAckMsg.RunID
	xl.AddPrefix(xlog.LogPrefix{Name: "runID", Value: svr.runID})

	xl.Info("login to server success, get run id [%s]", handshakeAckMsg.RunID)
	return
}

func (svr *Service) loopLoginUntilSuccess(maxInterval time.Duration, firstLoginExit bool) {
	xl := xlog.FromContextSafe(svr.ctx)

	loginFunc := func() (bool, error) {
		xl.Info("try to connect to server...")
		conn, connector, err := svr.login()
		if err != nil {
			xl.Warn("connect to server error: %v", err)
			if firstLoginExit {
				svr.cancel(cancelErr{Err: err})
			}
			return false, err
		}

		svr.cfgMu.RLock()
		forwardCfgs := svr.forwardCfgs
		visitorCfgs := svr.visitorCfgs
		svr.cfgMu.RUnlock()
		connEncrypted := true
		if svr.clientSpec != nil && svr.clientSpec.Type == "ssh-tunnel" {
			connEncrypted = false
		}
		sessionCtx := &SessionContext{
			Common:        svr.common,
			SessionID:     svr.runID,
			Conn:          conn,
			ConnEncrypted: connEncrypted,
			AuthSetter:    svr.authSetter,
			Connector:     connector,
		}
		ctl, err := NewControl(svr.ctx, sessionCtx)
		if err != nil {
			conn.Close()
			xl.Error("NewControl error: %v", err)
			return false, err
		}
		ctl.SetInWorkConnCallback(svr.handleWorkConnCb)

		ctl.Run(forwardCfgs, visitorCfgs)
		// close and replace previous control
		svr.ctlMu.Lock()
		if svr.ctl != nil {
			svr.ctl.Close()
		}
		svr.ctl = ctl
		svr.ctlMu.Unlock()
		return true, nil
	}

	// try to reconnect to server until success
	wait.BackoffUntil(loginFunc, wait.NewFastBackoffManager(
		wait.FastBackoffOptions{
			Duration:    time.Second,
			Factor:      2,
			Jitter:      0.1,
			MaxDuration: maxInterval,
		}), true, svr.ctx.Done())
}

func (svr *Service) UpdateAllConfigurer(
	forwardCfgs []v1.ForwardConfigurer,
	visitorCfgs []v1.VisitorConfigurer,
) error {
	svr.cfgMu.Lock()
	svr.forwardCfgs = forwardCfgs
	svr.visitorCfgs = visitorCfgs
	svr.cfgMu.Unlock()

	svr.ctlMu.RLock()
	ctl := svr.ctl
	svr.ctlMu.RUnlock()

	if ctl != nil {
		return svr.ctl.UpdateAllConfigurer(forwardCfgs, visitorCfgs)
	}
	return nil
}

func (svr *Service) Close() {
	svr.GracefulClose(time.Duration(0))
}

func (svr *Service) GracefulClose(d time.Duration) {
	svr.gracefulShutdownDuration = d
	svr.cancel(nil)
}

func (svr *Service) stop() {
	svr.ctlMu.Lock()
	defer svr.ctlMu.Unlock()
	if svr.ctl != nil {
		svr.ctl.GracefulClose(svr.gracefulShutdownDuration)
		svr.ctl = nil
	}
}

// TODO(vpp_team): Use StatusExporter to provide query interfaces instead of directly using methods from the Service.
func (svr *Service) GetForwardStatus(name string) (*forward.WorkingStatus, error) {
	svr.ctlMu.RLock()
	ctl := svr.ctl
	svr.ctlMu.RUnlock()

	if ctl == nil {
		return nil, fmt.Errorf("control is not running")
	}
	ws, ok := ctl.fm.GetForwardStatus(name)
	if !ok {
		return nil, fmt.Errorf("forward [%s] is not found", name)
	}
	return ws, nil
}
