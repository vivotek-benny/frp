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
	"reflect"

	v1 "monitoragent/pkg/config/v1"
)

func init() {
	fwdConfs := []v1.ForwardConfigurer{
		&v1.TCPForwardConfig{},
		&v1.HTTPForwardConfig{},
		&v1.HTTPSForwardConfig{},
		&v1.STCPForwardConfig{},
		&v1.TCPMuxForwardConfig{},
	}
	for _, cfg := range fwdConfs {
		RegisterForwardFactory(reflect.TypeOf(cfg), NewGeneralTCPForward)
	}
}

// GeneralTCPForward is a general implementation of Forward interface for TCP protocol.
// If the default GeneralTCPForward cannot meet the requirements, you can customize
// the implementation of the Forward interface.
type GeneralTCPForward struct {
	*BaseForward
}

func NewGeneralTCPForward(baseForward *BaseForward, _ v1.ForwardConfigurer) Forward {
	return &GeneralTCPForward{
		BaseForward: baseForward,
	}
}
