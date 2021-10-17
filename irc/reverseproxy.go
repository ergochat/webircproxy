// Copyright (c) 2021 Shivaram Lingamneni
// released under the MIT license

package irc

import (
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"

	"github.com/ergochat/irc-go/ircmsg"
	"github.com/ergochat/irc-go/ircreader"
	"github.com/gorilla/websocket"

	"github.com/ergochat/ergo/irc/utils"
)

const (
	initialBufferSize = 1024
)

var (
	crlf = []byte("\r\n")
)

func (server *Server) RunReverseProxyConn(webConn *websocket.Conn, proxiedIP net.IP, secure bool, config *Config) {
	ip := proxiedIP
	if ip == nil {
		ip = utils.AddrToIP(webConn.RemoteAddr())
	}
	ipString := utils.IPStringToHostname(ip.String())

	upstream := config.Upstreams[rand.Intn(len(config.Upstreams))]
	messageType := websocket.TextMessage
	if webConn.Subprotocol() == "binary.ircv3.net" {
		messageType = websocket.BinaryMessage
	}

	server.Log(LogLevelInfo, fmt.Sprintf("Received connection from %s, forwarding to %s", ipString, upstream.Address))

	var uConn net.Conn
	var err error
	proto := "tcp"
	if strings.HasPrefix(upstream.Address, "/") {
		proto = "unix"
	}
	if upstream.TLS {
		tlsConf := &tls.Config{
			ServerName:   upstream.Address,
			MinVersion:   tls.VersionTLS13,
			Certificates: upstream.Webirc.certificates,
		}
		uConn, err = tls.DialWithDialer(config.dialer, proto, upstream.Address, tlsConf)
	} else {
		uConn, err = config.dialer.Dial(proto, upstream.Address)
	}

	if err != nil {
		server.Log(LogLevelError, fmt.Sprintf("error connecting to upstream ircd at %s: %v", upstream.Address, err))
		webConn.Close()
		return
	}

	if upstream.Webirc.Enabled {
		var hostname string
		if config.LookupHostnames {
			hostname, _ = utils.LookupHostname(ip, config.ForwardConfirmHostnames)
		} else {
			hostname = ipString
		}
		flags := ""
		if secure {
			flags = "secure"
		}
		message := ircmsg.MakeMessage(nil, "", "WEBIRC",
			upstream.Webirc.Password, config.GatewayName, hostname, ipString, flags)
		messageBytes, err := message.LineBytesStrict(false, DefaultMaxLineLen)
		if err == nil {
			_, err = uConn.Write(messageBytes)
		}
		if err != nil {
			server.Log(LogLevelError, fmt.Sprintf("error sending WEBIRC to upstream at %s: %v", upstream.Address, err))
		} // but keep going
	}

	debug := config.logLevel >= LogLevelDebug
	NewReverseProxyConn(server, webConn, uConn, messageType, config.MaxLineLen, config.maxReadQBytes, debug)
}

type ReverseProxyConn struct {
	webConn     *websocket.Conn
	uConn       net.Conn
	messageType int

	closeOnce sync.Once

	server *Server
}

func NewReverseProxyConn(server *Server, webConn *websocket.Conn, uConn net.Conn, messageType int, maxLineLen, maxReadQ int, debug bool) *ReverseProxyConn {
	result := &ReverseProxyConn{
		webConn:     webConn,
		uConn:       uConn,
		messageType: messageType,
		server:      server,
	}
	go result.proxyToUpstream(debug)
	go result.proxyFromUpstream(maxLineLen, maxReadQ, debug)
	return result
}

func (r *ReverseProxyConn) proxyToUpstream(debug bool) {
	var errorMessage string
	defer func() {
		r.Close()
		r.server.Log(LogLevelInfo, errorMessage)
	}()

	buffers := make(net.Buffers, 2)
	for {
		_, line, err := r.webConn.ReadMessage()
		if err != nil {
			errorMessage = fmt.Sprintf("error reading from websocket conn at %s: %v", r.webConn.RemoteAddr().String(), err)
			return
		}
		if debug {
			r.server.Log(LogLevelDebug,
				fmt.Sprintf("input: %s -> %s: %s",
					r.webConn.RemoteAddr().String(), r.uConn.RemoteAddr().String(), line))
		}
		iovec := buffers
		iovec[0] = line
		iovec[1] = crlf
		// Go will optimize this to writev(2) if possible
		_, err = (&iovec).WriteTo(r.uConn)
		if err != nil {
			errorMessage = fmt.Sprintf("error writing to upstream conn at %s: %v", r.uConn.RemoteAddr().String(), err)
			return
		}
	}
}

func (r *ReverseProxyConn) proxyFromUpstream(maxLineLen, maxReadQ int, debug bool) {
	var errorMessage string
	defer func() {
		r.Close()
		r.server.Log(LogLevelInfo, errorMessage)
	}()

	// in case something sketchy happens in the chardet code:
	defer r.server.HandlePanic()

	var reader ircreader.Reader
	reader.Initialize(r.uConn, initialBufferSize, maxReadQ)
	for {
		// ircreader strips the \r\n:
		line, err := reader.ReadLine()
		if err != nil {
			errorMessage = fmt.Sprintf("error reading from upstream conn at %s: %v", r.uConn.RemoteAddr().String(), err)
			return
		}
		if debug {
			r.server.Log(LogLevelDebug,
				fmt.Sprintf("output: %s -> %s: %s",
					r.uConn.RemoteAddr().String(), r.webConn.RemoteAddr().String(), line))
		}
		if r.messageType == websocket.BinaryMessage {
			err = r.webConn.WriteMessage(websocket.BinaryMessage, line)
		} else {
			err = r.webConn.WriteMessage(websocket.TextMessage, r.server.transcodeToUTF8(line, maxLineLen))
		}
		if err != nil {
			errorMessage = fmt.Sprintf("error writing to websocket conn at %s: %v", r.webConn.RemoteAddr().String(), err)
			return
		}
	}
}

func (r *ReverseProxyConn) Close() {
	r.closeOnce.Do(r.realClose)
}

func (r *ReverseProxyConn) realClose() {
	r.webConn.Close()
	r.uConn.Close()
}
