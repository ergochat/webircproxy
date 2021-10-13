// Copyright (c) 2021 Shivaram Lingamneni
// released under the MIT license

package irc

import (
	"fmt"
	"runtime/debug"
)

func (server *Server) HandlePanic() {
	if r := recover(); r != nil {
		server.Log(LogLevelError, fmt.Sprintf("Panic encountered: %v\n%s", r, debug.Stack()))
	}
}
