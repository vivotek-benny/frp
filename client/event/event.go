package event

import (
	"errors"

	"monitoragent/pkg/msg"
)

var ErrPayloadType = errors.New("error payload type")

type Handler func(payload interface{}) error

type StartForwardPayload struct {
	NewForwardMsg *msg.NewForward
}

type CloseForwardPayload struct {
	CloseForwardMsg *msg.CloseForward
}
