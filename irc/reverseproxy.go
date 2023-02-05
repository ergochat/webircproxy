// Copyright (c) 2021 Shivaram Lingamneni
// released under the MIT license

package irc

import (
	"crypto/tls"
	"fmt"
	"io"
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

	server.Log(LogLevelInfo, fmt.Sprintf("received connection from %s, forwarding to %s", webConn.RemoteAddr(), upstream.Address))

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
	wsBuffer    []byte
	maxBuffer   int
	maxLineLen  int

	closeOnce sync.Once

	server *Server
}

func NewReverseProxyConn(server *Server, webConn *websocket.Conn, uConn net.Conn, messageType int, maxLineLen, maxReadQ int, debug bool) *ReverseProxyConn {
	result := &ReverseProxyConn{
		webConn:     webConn,
		uConn:       uConn,
		messageType: messageType,
		server:      server,
		wsBuffer:    make([]byte, initialBufferSize),
		maxBuffer:   maxReadQ,
		maxLineLen:  maxLineLen,
	}
	go result.proxyToUpstream(debug)
	go result.proxyFromUpstream(debug)
	return result
}

func (r *ReverseProxyConn) proxyToUpstream(debug bool) {
	var errorMessage string
	defer func() {
		r.Close()
		r.server.Log(LogLevelInfo, errorMessage)
	}()

	// XXX writev(2) / (*Buffers).WriteTo dance:
	// net.Buffers is [][]byte. first, allocate a slice of 2 []byte's, to hold
	// (1) the IRC line (2) the terminating CRLF
	buffers := make(net.Buffers, 2)
	// (*Buffers).WriteTo has two problems. (1) it destructively modifies the Buffers, i.e.
	// the [][]byte (although not the underlying byte sequences); after a successful WriteTo
	// the Buffers will contain an empty slice. so we need to keep a clean copy of
	// the `buffers` slice we just allocated, pointing to 2 []byte's.
	// (2) the net.Buffers that is the object of (*Buffers).WriteTo will escape to the heap;
	// this is a limitation of the escape analyzer. work around this by
	// preemptively allocating it a single time on the heap and reusing it:
	iovec := new(net.Buffers)
	for {
		line, err := r.readWSMessage()
		if err != nil {
			errorMessage = fmt.Sprintf("error reading from websocket conn at %s: %v", r.webConn.RemoteAddr().String(), err)
			return
		}
		if debug {
			r.server.Log(LogLevelDebug,
				fmt.Sprintf("input: %s -> %s: %s",
					r.webConn.RemoteAddr().String(), r.uConn.RemoteAddr().String(), line))
		}
		// step 1: reset *iovec to contain a slice of 2 []byte's:
		*iovec = buffers
		// step 2: fill in the two desired []byte's:
		(*iovec)[0] = line
		(*iovec)[1] = crlf
		// step 3: (*net.Buffers) prepared, Go will optimize this to writev(2) if possible:
		_, err = iovec.WriteTo(r.uConn)
		if err != nil {
			errorMessage = fmt.Sprintf("error writing to upstream conn at %s: %v", r.uConn.RemoteAddr().String(), err)
			return
		}
	}
}

func (r *ReverseProxyConn) readWSMessage() (line []byte, err error) {
	_, reader, err := r.webConn.NextReader()
	if err != nil {
		return nil, err
	}
	// XXX this is io.ReadFull with a single attempt to resize upwards
	n, err := io.ReadFull(reader, r.wsBuffer)
	if err == nil && len(r.wsBuffer) < r.maxBuffer {
		newBuf := make([]byte, r.maxBuffer)
		copy(newBuf, r.wsBuffer[:n])
		r.wsBuffer = newBuf
		var n2 int
		n2, err = io.ReadFull(reader, r.wsBuffer[n:])
		n += n2
	}
	line = r.wsBuffer[:n]
	switch err {
	case io.ErrUnexpectedEOF, io.EOF:
		// good: exhausted the reader without exhausting the buffer
		return line, nil
	case nil, websocket.ErrReadLimit:
		// bad: exhausted the buffer but the reader still has data
		return line, websocket.ErrReadLimit
	default:
		// bad: read error
		return line, err
	}
}

func (r *ReverseProxyConn) proxyFromUpstream(debug bool) {
	var errorMessage string
	defer func() {
		r.Close()
		r.server.Log(LogLevelInfo, errorMessage)
	}()

	// in case something sketchy happens in the chardet code:
	defer r.server.HandlePanic()

	var reader ircreader.Reader
	reader.Initialize(r.uConn, initialBufferSize, r.maxBuffer)
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
			err = r.webConn.WriteMessage(websocket.TextMessage, r.server.transcodeToUTF8(line, r.maxLineLen))
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
