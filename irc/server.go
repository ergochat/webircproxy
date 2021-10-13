// Copyright (c) 2021 Shivaram Lingamneni <slingamn@cs.stanford.edu>
// released under the MIT license

package irc

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/okzk/sdnotify"

	"github.com/ergochat/ergo/irc/utils"
)

// Server is the main Oragono server.
type Server struct {
	config         unsafe.Pointer
	configFilename string
	listeners      map[string]*WSListener
	rehashMutex    sync.Mutex // tier 4
	rehashSignal   chan os.Signal
	pprofServer    *http.Server
	exitSignals    chan os.Signal

	logMutex sync.Mutex
}

// NewServer returns a new Oragono server.
func NewServer(config *Config) (*Server, error) {
	// initialize data structures
	server := &Server{
		listeners:    make(map[string]*WSListener),
		rehashSignal: make(chan os.Signal, 1),
		exitSignals:  make(chan os.Signal, len(utils.ServerExitSignals)),
	}

	if err := server.applyConfig(config); err != nil {
		return nil, err
	}

	// Attempt to clean up when receiving these signals.
	signal.Notify(server.exitSignals, utils.ServerExitSignals...)
	signal.Notify(server.rehashSignal, syscall.SIGHUP)

	return server, nil
}

// Shutdown shuts down the server.
func (server *Server) Shutdown() {
	sdnotify.Stopping()
	server.Log(LogLevelInfo, "Exiting")
}

// Run starts the server.
func (server *Server) Run() {
	defer server.Shutdown()

	for {
		select {
		case <-server.exitSignals:
			return
		case <-server.rehashSignal:
			go server.rehash()
		}
	}
}

type LogLevel uint

const (
	LogLevelError LogLevel = iota
	LogLevelWarn
	LogLevelInfo
	LogLevelDebug
)

func (server *Server) Log(level LogLevel, message string) {
	if level <= server.Config().logLevel {
		server.logMutex.Lock()
		fmt.Fprintf(os.Stderr, "%s [%s] %s\n",
			logLevelToString(level), time.Now().UTC().Format(utils.IRCv3TimestampFormat), message)
		server.logMutex.Unlock()
	}
}

//
// server functionality
//

// rehash reloads the config and applies the changes from the config file.
func (server *Server) rehash() error {
	defer server.HandlePanic()

	server.Log(LogLevelInfo, "Attempting rehash")

	// only let one REHASH go on at a time
	server.rehashMutex.Lock()
	defer server.rehashMutex.Unlock()

	sdnotify.Reloading()
	defer sdnotify.Ready()

	config, err := LoadConfig(server.configFilename)
	if err != nil {
		server.Log(LogLevelError, fmt.Sprintf("Failed to load config file: %v", err.Error()))
		return err
	}

	err = server.applyConfig(config)
	if err != nil {
		server.Log(LogLevelError, fmt.Sprintf("Failed to rehash: %v", err.Error()))
		return err
	}

	server.Log(LogLevelInfo, "Rehash completed successfully")
	return nil
}

func (server *Server) applyConfig(config *Config) (err error) {
	oldConfig := server.Config()
	initial := oldConfig == nil

	if initial {
		server.configFilename = config.Filename
	}

	// activate the new config
	server.SetConfig(config)

	server.Log(LogLevelInfo, fmt.Sprintf("Using config file %s", server.configFilename))

	server.setupPprofListener(config)

	// we are now ready to receive connections:
	err = server.setupListeners(config)

	if initial && err == nil {
		server.Log(LogLevelInfo, "Server running")
		sdnotify.Ready()
	}

	return err
}

func (server *Server) setupPprofListener(config *Config) {
	pprofListener := config.PprofListener
	if server.pprofServer != nil {
		if pprofListener == "" || (pprofListener != server.pprofServer.Addr) {
			server.Log(LogLevelInfo, fmt.Sprintf("Stopping pprof listener at %s", server.pprofServer.Addr))
			server.pprofServer.Close()
			server.pprofServer = nil
		}
	}
	if pprofListener != "" && server.pprofServer == nil {
		ps := http.Server{
			Addr: pprofListener,
		}
		go func() {
			if err := ps.ListenAndServe(); err != nil {
				server.Log(LogLevelError, fmt.Sprintf("pprof listener failed: %v", err))
			}
		}()
		server.pprofServer = &ps
		server.Log(LogLevelInfo, fmt.Sprintf("Started pprof listener: %s", server.pprofServer.Addr))
	}
}

func (server *Server) setupListeners(config *Config) (err error) {
	logListener := func(addr string, config utils.ListenerConfig) {
		server.Log(LogLevelInfo,
			fmt.Sprintf("now listening on %s, tls=%t, proxy=%t, tor=%t", addr, (config.TLSConfig != nil), config.RequireProxy, config.Tor),
		)
	}

	// update or destroy all existing listeners
	for addr := range server.listeners {
		currentListener := server.listeners[addr]
		newConfig, stillConfigured := config.trueListeners[addr]

		if stillConfigured {
			if reloadErr := currentListener.Reload(newConfig); reloadErr == nil {
				logListener(addr, newConfig)
			} else {
				// stop the listener; we will attempt to replace it below
				currentListener.Stop()
				delete(server.listeners, addr)
			}
		} else {
			currentListener.Stop()
			delete(server.listeners, addr)
			server.Log(LogLevelInfo, fmt.Sprintf("stopped listening on %s.", addr))
		}
	}

	// create new listeners that were not previously configured,
	// or that couldn't be reloaded above:
	for newAddr, newConfig := range config.trueListeners {
		_, exists := server.listeners[newAddr]
		if !exists {
			// make a new listener
			newListener, newErr := NewListener(server, newAddr, newConfig, config.UnixBindMode)
			if newErr == nil {
				server.listeners[newAddr] = newListener
				logListener(newAddr, newConfig)
			} else {
				server.Log(LogLevelInfo, fmt.Sprintf("couldn't listen on %s: %v", newAddr, newErr))
				err = newErr
			}
		}
	}

	return
}
