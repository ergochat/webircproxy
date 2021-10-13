// Copyright (c) 2021 Shivaram Lingamneni
// released under the MIT license

package irc

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ergochat/irc-go/ircmsg"
	"github.com/gogs/chardet"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/ianaindex"
)

var (
	InvalidIRCSyntax = errors.New("invalid IRC syntax")
)

func invalidMessageWarning() []byte {
	return []byte("WARN * INVALID_MESSAGE :Upstream server sent a syntactically invalid message")
}

// Transcode a raw IRC line (without \r\n) to UTF-8, without introducing any new
// protocol violations.
func (server *Server) transcodeToUTF8(line []byte, maxLineLen int) (result []byte) {
	if utf8.Valid(line) {
		return line
	}

	config := server.Config()
	if config.Transcoding.EnableChardet {
		return server.decodeViaParamTranscoding(line, maxLineLen, func(param string) string {
			return server.decodeParamViaChardet(config.Transcoding.detector, param)
		})
	} else if len(config.Transcoding.encodings) != 0 {
		return server.decodeViaParamTranscoding(line, maxLineLen, func(param string) string {
			return server.decodeParamViaEncodingList(param, config.Transcoding.encodings)
		})
	} else {
		return server.decodeViaReplacementRune(line, maxLineLen)
	}
}

// transcode message to UTF-8, replace any invalid sequences with the Unicode
// replacement character, be as efficient as possible
func (server *Server) decodeViaReplacementRune(line []byte, maxLineLen int) (result []byte) {
	var out bytes.Buffer

	// include the tags portion verbatim if it's present, since valid tag data
	// is always utf-8:
	if line[0] == '@' {
		spaceIdx := bytes.IndexByte(line, ' ')
		if spaceIdx != -1 {
			out.Write(line[:spaceIdx+1])
			line = line[spaceIdx+1:]
		} else {
			// IRC lines MUST contain a command; this message is invalid
			return invalidMessageWarning()
		}
	}

	// using the replacement character can increase the byte length of a string;
	// make sure we don't increase the line length over the valid limit.
	// (if it was over the limit when we found it, correcting that is out of scope)
	bodyLength := 0
	for len(line) != 0 {
		r, l := utf8.DecodeRune(line)
		if r != utf8.RuneError {
			if bodyLength+l <= (maxLineLen - 2) {
				out.Write(line[:l])
				line = line[l:]
			} else {
				break
			}
		} else {
			// use the unicode replacement character, '\uFFFD';
			// its UTF8 encoding is 3 bytes, '\xef\xbf\xbd'
			if bodyLength+3 <= (maxLineLen - 2) {
				out.WriteString("\xef\xbf\xbd")
				line = line[1:]
			} else {
				break
			}
		}
	}
	return out.Bytes()
}

// Transcode an IRC line to UTF-8 via the following algorithm:
// 1. Parse the line as IRC
// 2. Transcode each parameter individually, using a pluggable transcoding function
// 3. Reserialize the line
func (server *Server) decodeViaParamTranscoding(line []byte, maxLineLen int, paramTranscoder func(string) string) (result []byte) {
	msg, err := ircmsg.ParseLine(string(line))
	if err != nil {
		server.Log(LogLevelWarn, fmt.Sprintf("invalid message from upstream: %v", err))
		return invalidMessageWarning()
	}

	// tags are always valid utf8 (and ircmsg validates this)
	// prefix is a black box, but just assume utf8 and hope for the best
	// command MUST be utf8
	if !utf8.ValidString(msg.Prefix) {
		msg.Prefix = decodeAsUtf8(msg.Prefix)
	}
	if !utf8.ValidString(msg.Command) {
		server.Log(LogLevelWarn, fmt.Sprintf("invalid command from upstream: %v", []byte(msg.Command)))
		return invalidMessageWarning()
	}
	// transcode each parameter individually
	for i := range msg.Params {
		msg.Params[i] = paramTranscoder(msg.Params[i])
	}

	out, err := msg.LineBytesStrict(false, maxLineLen)
	if err != nil && err != ircmsg.ErrorBodyTooLong {
		server.Log(LogLevelWarn, fmt.Sprintf("error reassembling message after transcoding: %v", err))
		return invalidMessageWarning()
	}
	out = bytes.TrimSuffix(out, crlf)
	return out
}

func (server *Server) decodeParamViaChardet(detector *chardet.Detector, param string) (result string) {
	if utf8.ValidString(param) {
		return param
	}

	det, err := detector.DetectBest([]byte(param))
	if err != nil {
		server.Log(LogLevelWarn, fmt.Sprintf("chardet failed: %v", err))
		return decodeAsUtf8(param)
	}

	enc, err := ianaindex.IANA.Encoding(det.Charset)
	if err != nil {
		server.Log(LogLevelWarn, fmt.Sprintf("chardet returned unknown charset %s: %v", det.Charset, err))
		return decodeAsUtf8(param)
	}

	decoded, err := enc.NewDecoder().String(param)
	if err != nil {
		server.Log(LogLevelWarn, fmt.Sprintf("chardet detected charset %s but could not decode: %v", det.Charset, err))
		return decodeAsUtf8(param)
	}

	return decoded
}

func (server *Server) decodeParamViaEncodingList(param string, encodings []encoding.Encoding) (result string) {
	for _, encoding := range encodings {
		decoded, err := encoding.NewDecoder().String(param)
		if err == nil {
			return decoded
		}
	}
	return decodeAsUtf8(param)
}

// XXX is this really the best way to do this?
func decodeAsUtf8(param string) (result string) {
	var out strings.Builder
	for _, r := range param {
		out.WriteRune(r)
	}
	return out.String()
}
