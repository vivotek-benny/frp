package client

import (
	"monitoragent/client/connector"
	"monitoragent/pkg/auth"
	v1 "monitoragent/pkg/config/v1"
	"net"
)

type SessionContext struct {
	// The client common configuration.
	Common *v1.ClientCommonConfig

	// Unique ID obtained from monitoragents.
	// It should be attached to the login message when reconnecting.
	SessionID string
	// Underlying control connection. Once conn is closed, the msgDispatcher and the entire Control will exit.
	Conn net.Conn
	// Indicates whether the connection is encrypted.
	ConnEncrypted bool
	// Sets authentication based on selected method
	AuthSetter auth.Setter
	// Connector is used to create new connections, which could be real TCP connections or virtual streams.
	Connector connector.Connector
}
