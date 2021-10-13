// Copyright (c) 2021 Shivaram Lingamneni <slingamn@cs.stanford.edu>
// released under the MIT license

package irc

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ergochat/ergo/irc/utils"
)

var (
	errCantReloadListener = errors.New("can't switch a listener between stream and websocket")
)

// NewListener creates a new listener according to the specifications in the config file
func NewListener(server *Server, addr string, config utils.ListenerConfig, bindMode os.FileMode) (result *WSListener, err error) {
	baseListener, err := createBaseListener(addr, bindMode)
	if err != nil {
		return
	}

	wrappedListener := utils.NewReloadableListener(baseListener, config)

	return NewWSListener(server, addr, wrappedListener, config)
}

func createBaseListener(addr string, bindMode os.FileMode) (listener net.Listener, err error) {
	addr = strings.TrimPrefix(addr, "unix:")
	if strings.HasPrefix(addr, "/") {
		// https://stackoverflow.com/a/34881585
		os.Remove(addr)
		listener, err = net.Listen("unix", addr)
		if err == nil && bindMode != 0 {
			os.Chmod(addr, bindMode)
		}
	} else {
		listener, err = net.Listen("tcp", addr)
	}
	return
}

// WSListener is a listener for IRC-over-websockets (initially HTTP, then upgraded to a
// different application protocol that provides a message-based API, possibly with TLS)
type WSListener struct {
	listener   *utils.ReloadableListener
	httpServer *http.Server
	server     *Server
	addr       string
}

func NewWSListener(server *Server, addr string, listener *utils.ReloadableListener, config utils.ListenerConfig) (result *WSListener, err error) {
	result = &WSListener{
		listener: listener,
		server:   server,
		addr:     addr,
	}
	result.httpServer = &http.Server{
		Handler:      http.HandlerFunc(result.handle),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	go result.httpServer.Serve(listener)
	return
}

func (wl *WSListener) Reload(config utils.ListenerConfig) error {
	wl.listener.Reload(config)
	return nil
}

func (wl *WSListener) Stop() error {
	return wl.httpServer.Close()
}

func (wl *WSListener) handle(w http.ResponseWriter, r *http.Request) {
	config := wl.server.Config()
	remoteAddr := r.RemoteAddr
	xff := r.Header.Get("X-Forwarded-For")
	xfp := r.Header.Get("X-Forwarded-Proto")

	wsUpgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			if len(config.allowedOriginRegexps) == 0 {
				return true
			}
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if len(origin) == 0 {
				return false
			}
			for _, re := range config.allowedOriginRegexps {
				if re.MatchString(origin) {
					return true
				}
			}
			return false
		},
		Subprotocols: []string{"text.ircv3.net", "binary.ircv3.net"},
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		wl.server.Log(LogLevelInfo, fmt.Sprintf("websocket upgrade error from %s: %v", wl.addr, err))
		return
	}

	wConn, ok := conn.UnderlyingConn().(*utils.WrappedConn)
	if !ok {
		wl.server.Log(LogLevelInfo, fmt.Sprintf("non-proxied connection from: %s", wl.addr))
		conn.Close()
		return
	}

	confirmProxyData(wConn, remoteAddr, xff, xfp, config)

	// avoid a DoS attack from buffering excessively large messages:
	conn.SetReadLimit(int64(config.maxReadQBytes))

	go wl.server.RunReverseProxyConn(conn, wConn.ProxiedIP, wConn.Secure, config)
}

// validate conn.ProxiedIP and conn.Secure against config, HTTP headers, etc.
func confirmProxyData(conn *utils.WrappedConn, remoteAddr, xForwardedFor, xForwardedProto string, config *Config) {
	if conn.ProxiedIP != nil {
		if !utils.IPInNets(utils.AddrToIP(conn.RemoteAddr()), config.proxyAllowedFromNets) {
			conn.ProxiedIP = nil
		}
	} else if xForwardedFor != "" {
		proxiedIP := utils.HandleXForwardedFor(remoteAddr, xForwardedFor, config.proxyAllowedFromNets)
		// don't set proxied IP if it is redundant with the actual IP
		if proxiedIP != nil && !proxiedIP.Equal(utils.AddrToIP(conn.RemoteAddr())) {
			conn.ProxiedIP = proxiedIP
		}
	}

	if conn.Config.TLSConfig != nil || conn.Config.Tor {
		// we terminated our own encryption:
		conn.Secure = true
	} else {
		// plaintext websocket: trust X-Forwarded-Proto from a trusted source
		conn.Secure = utils.IPInNets(utils.AddrToIP(conn.RemoteAddr()), config.proxyAllowedFromNets) &&
			xForwardedProto == "https"
	}
}
