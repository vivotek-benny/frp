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
	"reflect"
	"sync"

	"github.com/samber/lo"

	"monitoragent/client/event"
	v1 "monitoragent/pkg/config/v1"
	"monitoragent/pkg/msg"
	"monitoragent/pkg/transport"
	"monitoragent/pkg/util/xlog"
)

type Manager struct {
	proxies            map[string]*Wrapper
	msgTransporter     transport.MessageTransporter
	inWorkConnCallback func(*v1.ForwardBaseConfig, net.Conn, *msg.StartWorkConn) bool

	closed bool
	mu     sync.RWMutex

	clientCfg *v1.ClientCommonConfig

	ctx context.Context
}

func NewManager(
	ctx context.Context,
	clientCfg *v1.ClientCommonConfig,
	msgTransporter transport.MessageTransporter,
) *Manager {
	return &Manager{
		proxies:        make(map[string]*Wrapper),
		msgTransporter: msgTransporter,
		closed:         false,
		clientCfg:      clientCfg,
		ctx:            ctx,
	}
}

func (pm *Manager) StartForward(name string, remoteAddr string, serverRespErr string) error {
	pm.mu.RLock()
	fwd, ok := pm.proxies[name]
	pm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("forward [%s] not found", name)
	}

	err := fwd.SetRunningStatus(remoteAddr, serverRespErr)
	if err != nil {
		return err
	}
	return nil
}

func (pm *Manager) SetInWorkConnCallback(
	cb func(*v1.ForwardBaseConfig, net.Conn, *msg.StartWorkConn) bool,
) {
	pm.inWorkConnCallback = cb
}

func (pm *Manager) Close() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, fwd := range pm.proxies {
		fwd.Stop()
	}
	pm.proxies = make(map[string]*Wrapper)
}

func (pm *Manager) HandleWorkConn(name string, workConn net.Conn, m *msg.StartWorkConn) {
	pm.mu.RLock()
	pw, ok := pm.proxies[name]
	pm.mu.RUnlock()
	if ok {
		pw.InWorkConn(workConn, m)
	} else {
		workConn.Close()
	}
}

func (pm *Manager) HandleEvent(payload interface{}) error {
	var m msg.Message
	switch e := payload.(type) {
	case *event.StartForwardPayload:
		m = e.NewForwardMsg
	case *event.CloseForwardPayload:
		m = e.CloseForwardMsg
	default:
		return event.ErrPayloadType
	}

	return pm.msgTransporter.Send(m)
}

func (pm *Manager) GetAllForwardStatus() []*WorkingStatus {
	ps := make([]*WorkingStatus, 0)
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, fwd := range pm.proxies {
		ps = append(ps, fwd.GetStatus())
	}
	return ps
}

func (pm *Manager) GetForwardStatus(name string) (*WorkingStatus, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if fwd, ok := pm.proxies[name]; ok {
		return fwd.GetStatus(), true
	}
	return nil, false
}

func (pm *Manager) UpdateAll(forwardCfgs []v1.ForwardConfigurer) {
	xl := xlog.FromContextSafe(pm.ctx)
	forwardCfgsMap := lo.KeyBy(forwardCfgs, func(c v1.ForwardConfigurer) string {
		return c.GetBaseConfig().Name
	})
	pm.mu.Lock()
	defer pm.mu.Unlock()

	delFwdNames := make([]string, 0)
	for name, fwd := range pm.proxies {
		del := false
		cfg, ok := forwardCfgsMap[name]
		if !ok || !reflect.DeepEqual(fwd.Cfg, cfg) {
			del = true
		}

		if del {
			delFwdNames = append(delFwdNames, name)
			delete(pm.proxies, name)
			fwd.Stop()
		}
	}
	if len(delFwdNames) > 0 {
		xl.Info("forward removed: %s", delFwdNames)
	}

	addFwdNames := make([]string, 0)
	for _, cfg := range forwardCfgs {
		name := cfg.GetBaseConfig().Name
		if _, ok := pm.proxies[name]; !ok {
			fwd := NewWrapper(pm.ctx, cfg, pm.clientCfg, pm.HandleEvent, pm.msgTransporter)
			if pm.inWorkConnCallback != nil {
				fwd.SetInWorkConnCallback(pm.inWorkConnCallback)
			}
			pm.proxies[name] = fwd
			addFwdNames = append(addFwdNames, name)

			fwd.Start()
		}
	}
	if len(addFwdNames) > 0 {
		xl.Info("forward added: %s", addFwdNames)
	}
}
