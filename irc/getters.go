// Copyright (c) 2021 Shivaram Lingamneni <slingamn@cs.stanford.edu>
// released under the MIT license

package irc

import (
	"sync/atomic"
	"unsafe"
)

func (server *Server) Config() (config *Config) {
	return (*Config)(atomic.LoadPointer(&server.config))
}

func (server *Server) SetConfig(config *Config) {
	atomic.StorePointer(&server.config, unsafe.Pointer(config))
}
