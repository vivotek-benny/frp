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

package forward

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatedier/golib/errors"

	"monitoragent/client/event"
	"monitoragent/client/health"
	v1 "monitoragent/pkg/config/v1"
	"monitoragent/pkg/msg"
	"monitoragent/pkg/transport"
	"monitoragent/pkg/util/xlog"
)

const (
	ForwardPhaseNew         = "new"
	ForwardPhaseWaitStart   = "wait start"
	ForwardPhaseStartErr    = "start error"
	ForwardPhaseRunning     = "running"
	ForwardPhaseCheckFailed = "check failed"
	ForwardPhaseClosed      = "closed"
)

var (
	statusCheckInterval = 3 * time.Second
	waitResponseTimeout = 20 * time.Second
	startErrTimeout     = 30 * time.Second
)

type WorkingStatus struct {
	Name  string               `json:"name"`
	Type  string               `json:"type"`
	Phase string               `json:"status"`
	Err   string               `json:"err"`
	Cfg   v1.ForwardConfigurer `json:"cfg"`

	// Got from server.
	RemoteAddr string `json:"remote_addr"`
}

type Wrapper struct {
	WorkingStatus

	// underlying forward
	fwd Forward

	// if ForwardConf has healcheck config
	// monitor will watch if it is alive
	monitor *health.Monitor

	// event handler
	handler event.Handler

	msgTransporter transport.MessageTransporter

	health           uint32
	lastSendStartMsg time.Time
	lastStartErr     time.Time
	closeCh          chan struct{}
	healthNotifyCh   chan struct{}
	mu               sync.RWMutex

	xl  *xlog.Logger
	ctx context.Context
}

func NewWrapper(
	ctx context.Context,
	cfg v1.ForwardConfigurer,
	clientCfg *v1.ClientCommonConfig,
	eventHandler event.Handler,
	msgTransporter transport.MessageTransporter,
) *Wrapper {
	baseInfo := cfg.GetBaseConfig()
	xl := xlog.FromContextSafe(ctx).Spawn().AppendPrefix(baseInfo.Name)
	pw := &Wrapper{
		WorkingStatus: WorkingStatus{
			Name:  baseInfo.Name,
			Type:  baseInfo.Type,
			Phase: ForwardPhaseNew,
			Cfg:   cfg,
		},
		closeCh:        make(chan struct{}),
		healthNotifyCh: make(chan struct{}),
		handler:        eventHandler,
		msgTransporter: msgTransporter,
		xl:             xl,
		ctx:            xlog.NewContext(ctx, xl),
	}

	if baseInfo.HealthCheck.Type != "" && baseInfo.LocalPort > 0 {
		pw.health = 1 // means failed
		addr := net.JoinHostPort(baseInfo.LocalIP, strconv.Itoa(baseInfo.LocalPort))
		pw.monitor = health.NewMonitor(pw.ctx, baseInfo.HealthCheck, addr,
			pw.statusNormalCallback, pw.statusFailedCallback)
		xl.Trace("enable health check monitor")
	}

	pw.fwd = NewForward(pw.ctx, pw.Cfg, clientCfg, pw.msgTransporter)
	return pw
}

func (pw *Wrapper) SetInWorkConnCallback(
	cb func(*v1.ForwardBaseConfig, net.Conn, *msg.StartWorkConn) bool,
) {
	pw.fwd.SetInWorkConnCallback(cb)
}

func (pw *Wrapper) SetRunningStatus(remoteAddr string, respErr string) error {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	if pw.Phase != ForwardPhaseWaitStart {
		return fmt.Errorf("status not wait start, ignore start message")
	}

	pw.RemoteAddr = remoteAddr
	if respErr != "" {
		pw.Phase = ForwardPhaseStartErr
		pw.Err = respErr
		pw.lastStartErr = time.Now()
		return fmt.Errorf(pw.Err)
	}

	if err := pw.fwd.Run(); err != nil {
		pw.close()
		pw.Phase = ForwardPhaseStartErr
		pw.Err = err.Error()
		pw.lastStartErr = time.Now()
		return err
	}

	pw.Phase = ForwardPhaseRunning
	pw.Err = ""
	return nil
}

func (pw *Wrapper) Start() {
	go pw.checkWorker()
	if pw.monitor != nil {
		go pw.monitor.Start()
	}
}

func (pw *Wrapper) Stop() {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	close(pw.closeCh)
	close(pw.healthNotifyCh)
	pw.fwd.Close()
	if pw.monitor != nil {
		pw.monitor.Stop()
	}
	pw.Phase = ForwardPhaseClosed
	pw.close()
}

func (pw *Wrapper) close() {
	_ = pw.handler(&event.CloseForwardPayload{
		CloseForwardMsg: &msg.CloseForward{
			ForwardName: pw.Name,
		},
	})
}

func (pw *Wrapper) checkWorker() {
	xl := pw.xl
	if pw.monitor != nil {
		// let monitor do check request first
		time.Sleep(500 * time.Millisecond)
	}
	for {
		// check forward status
		now := time.Now()
		if atomic.LoadUint32(&pw.health) == 0 {
			pw.mu.Lock()
			if pw.Phase == ForwardPhaseNew ||
				pw.Phase == ForwardPhaseCheckFailed ||
				(pw.Phase == ForwardPhaseWaitStart && now.After(pw.lastSendStartMsg.Add(waitResponseTimeout))) ||
				(pw.Phase == ForwardPhaseStartErr && now.After(pw.lastStartErr.Add(startErrTimeout))) {

				xl.Trace("change status from [%s] to [%s]", pw.Phase, ForwardPhaseWaitStart)
				pw.Phase = ForwardPhaseWaitStart

				var newForwardMsg msg.NewForward
				pw.Cfg.MarshalToMsg(&newForwardMsg)
				pw.lastSendStartMsg = now
				_ = pw.handler(&event.StartForwardPayload{
					NewForwardMsg: &newForwardMsg,
				})
			}
			pw.mu.Unlock()
		} else {
			pw.mu.Lock()
			if pw.Phase == ForwardPhaseRunning || pw.Phase == ForwardPhaseWaitStart {
				pw.close()
				xl.Trace("change status from [%s] to [%s]", pw.Phase, ForwardPhaseCheckFailed)
				pw.Phase = ForwardPhaseCheckFailed
			}
			pw.mu.Unlock()
		}

		select {
		case <-pw.closeCh:
			return
		case <-time.After(statusCheckInterval):
		case <-pw.healthNotifyCh:
		}
	}
}

func (pw *Wrapper) statusNormalCallback() {
	xl := pw.xl
	atomic.StoreUint32(&pw.health, 0)
	_ = errors.PanicToError(func() {
		select {
		case pw.healthNotifyCh <- struct{}{}:
		default:
		}
	})
	xl.Info("health check success")
}

func (pw *Wrapper) statusFailedCallback() {
	xl := pw.xl
	atomic.StoreUint32(&pw.health, 1)
	_ = errors.PanicToError(func() {
		select {
		case pw.healthNotifyCh <- struct{}{}:
		default:
		}
	})
	xl.Info("health check failed")
}

func (pw *Wrapper) InWorkConn(workConn net.Conn, m *msg.StartWorkConn) {
	xl := pw.xl
	pw.mu.RLock()
	fwd := pw.fwd
	pw.mu.RUnlock()
	if fwd != nil && pw.Phase == ForwardPhaseRunning {
		xl.Debug(
			"start a new work connection, localAddr: %s remoteAddr: %s",
			workConn.LocalAddr().String(),
			workConn.RemoteAddr().String(),
		)
		go fwd.InWorkConn(workConn, m)
	} else {
		workConn.Close()
	}
}

func (pw *Wrapper) GetStatus() *WorkingStatus {
	pw.mu.RLock()
	defer pw.mu.RUnlock()
	ps := &WorkingStatus{
		Name:       pw.Name,
		Type:       pw.Type,
		Phase:      pw.Phase,
		Err:        pw.Err,
		Cfg:        pw.Cfg,
		RemoteAddr: pw.RemoteAddr,
	}
	return ps
}
