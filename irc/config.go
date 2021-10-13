// Copyright (c) 2021 Shivaram Lingamneni <slingamn@cs.stanford.edu>
// released under the MIT license

package irc

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ergochat/irc-go/ircmsg"
	"github.com/gogs/chardet"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/ianaindex"
	"gopkg.in/yaml.v2"

	"github.com/ergochat/ergo/irc/utils"
)

const (
	DefaultMaxLineLen = 512
)

// here's how this works: exported (capitalized) members of the config structs
// are defined in the YAML file and deserialized directly from there. They may
// be postprocessed and overwritten by LoadConfig. Unexported (lowercase) members
// are derived from the exported members in LoadConfig.

// TLSListenConfig defines configuration options for listening on TLS.
type TLSListenConfig struct {
	Cert string
	Key  string
}

// This is the YAML-deserializable type of the value of the `Server.Listeners` map
type listenerConfigBlock struct {
	// normal TLS configuration, with a single certificate:
	TLS TLSListenConfig
	// SNI configuration, with multiple certificates:
	TLSCertificates []TLSListenConfig `yaml:"tls-certificates"`
	MinTLSVersion   string            `yaml:"min-tls-version"`
	Proxy           bool
	Tor             bool
	STSOnly         bool `yaml:"sts-only"`
}

type reverseProxyUpstream struct {
	Address string
	TLS     bool `yaml:"tls"`
	Webirc  struct {
		Enabled      bool
		Password     string
		Cert         string
		Key          string
		certificates []tls.Certificate
	}
}

// Config defines the overall configuration.
type Config struct {
	Listeners    map[string]listenerConfigBlock
	UnixBindMode os.FileMode `yaml:"unix-bind-mode"`

	// they get parsed into this internal representation:
	trueListeners map[string]utils.ListenerConfig

	GatewayName string `yaml:"gateway-name"`
	dialer      *net.Dialer
	Upstreams   []reverseProxyUpstream
	DialTimeout time.Duration `yaml:"dial-timeout"`

	LookupHostnames         bool `yaml:"lookup-hostnames"`
	ForwardConfirmHostnames bool `yaml:"forward-confirm-hostnames"`

	ProxyAllowedFrom     []string `yaml:"proxy-allowed-from"`
	proxyAllowedFromNets []net.IPNet

	MaxLineLen    int `yaml:"max-line-len"`
	maxReadQBytes int

	AllowedOrigins       []string `yaml:"allowed-origins"`
	allowedOriginRegexps []*regexp.Regexp

	PprofListener string `yaml:"pprof-listener"`

	LogLevel string `yaml:"log-level"`
	logLevel LogLevel

	Transcoding struct {
		EnableChardet bool `yaml:"enable-chardet"`
		detector      *chardet.Detector
		Encodings     []string
		encodings     []encoding.Encoding
	}

	Filename string
}

func loadTlsConfig(config listenerConfigBlock) (tlsConfig *tls.Config, err error) {
	var certificates []tls.Certificate
	if len(config.TLSCertificates) != 0 {
		// SNI configuration with multiple certificates
		for _, certPairConf := range config.TLSCertificates {
			cert, err := loadCertWithLeaf(certPairConf.Cert, certPairConf.Key)
			if err != nil {
				return nil, err
			}
			certificates = append(certificates, cert)
		}
	} else if config.TLS.Cert != "" {
		// normal configuration with one certificate
		cert, err := loadCertWithLeaf(config.TLS.Cert, config.TLS.Key)
		if err != nil {
			return nil, err
		}
		certificates = append(certificates, cert)
	} else {
		// plaintext!
		return nil, nil
	}
	// if Chrome receives a server request for a client certificate
	// on a websocket connection, it will immediately disconnect:
	// https://bugs.chromium.org/p/chromium/issues/detail?id=329884
	// work around this behavior:
	clientAuth := tls.NoClientCert
	result := tls.Config{
		Certificates: certificates,
		ClientAuth:   clientAuth,
		MinVersion:   tlsMinVersionFromString(config.MinTLSVersion),
	}
	return &result, nil
}

func tlsMinVersionFromString(version string) uint16 {
	version = strings.ToLower(version)
	version = strings.TrimPrefix(version, "v")
	switch version {
	case "1", "1.0":
		return tls.VersionTLS10
	case "1.1":
		return tls.VersionTLS11
	case "1.2":
		return tls.VersionTLS12
	case "1.3":
		return tls.VersionTLS13
	default:
		// tls package will fill in a sane value, currently 1.0
		return tls.VersionTLS12
	}
}

func loadCertWithLeaf(certFile, keyFile string) (cert tls.Certificate, err error) {
	// LoadX509KeyPair: "On successful return, Certificate.Leaf will be nil because
	// the parsed form of the certificate is not retained." tls.Config:
	// "Note: if there are multiple Certificates, and they don't have the
	// optional field Leaf set, certificate selection will incur a significant
	// per-handshake performance cost."
	cert, err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return
	}
	cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
	return
}

// prepareListeners populates Config.Server.trueListeners
func (conf *Config) prepareListeners() (err error) {
	if len(conf.Listeners) == 0 {
		return fmt.Errorf("No listeners were configured")
	}

	conf.trueListeners = make(map[string]utils.ListenerConfig)
	for addr, block := range conf.Listeners {
		var lconf utils.ListenerConfig
		lconf.ProxyDeadline = time.Minute
		lconf.Tor = block.Tor
		lconf.TLSConfig, err = loadTlsConfig(block)
		if err != nil {
			return err
		}
		lconf.RequireProxy = block.Proxy
		conf.trueListeners[addr] = lconf
	}
	return nil
}

// LoadRawConfig loads the config without doing any consistency checks or postprocessing
func LoadRawConfig(filename string) (config *Config, err error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return
}

func parseLogLevel(str string) LogLevel {
	switch strings.ToLower(str) {
	case "error":
		return LogLevelError
	case "warn", "warning":
		return LogLevelWarn
	case "info":
		return LogLevelInfo
	case "debug":
		return LogLevelDebug
	default:
		return LogLevelInfo
	}
}

func logLevelToString(level LogLevel) string {
	switch level {
	case LogLevelError:
		return "[error]"
	case LogLevelWarn:
		return "[ warn]"
	case LogLevelInfo:
		return "[ info]"
	case LogLevelDebug:
		return "[debug]"
	default:
		return "[error]"
	}
}

// LoadConfig loads the given YAML configuration file.
func LoadConfig(filename string) (config *Config, err error) {
	config, err = LoadRawConfig(filename)
	if err != nil {
		return nil, err
	}
	config.Filename = filename
	return postprocessConfig(config)
}

func postprocessConfig(c *Config) (config *Config, err error) {
	config = c

	if config.GatewayName != utils.SafeErrorParam(config.GatewayName) {
		return nil, fmt.Errorf("gateway name must be valid as a non-final IRC parameter: nonempty, no spaces, no initial :")
	}

	config.logLevel = parseLogLevel(config.LogLevel)

	if config.MaxLineLen < DefaultMaxLineLen {
		config.MaxLineLen = DefaultMaxLineLen
	}
	config.maxReadQBytes = ircmsg.MaxlenClientTagData + config.MaxLineLen + 1024

	err = config.prepareListeners()
	if err != nil {
		return nil, fmt.Errorf("failed to prepare listeners: %v", err)
	}

	if config.DialTimeout == 0 {
		config.DialTimeout = 5 * time.Second
	}
	config.dialer = &net.Dialer{
		Timeout: config.DialTimeout,
	}

	if len(config.Upstreams) == 0 {
		return nil, fmt.Errorf("no upstreams configured")
	}

	for i, upstream := range config.Upstreams {
		config.Upstreams[i].Address = strings.TrimPrefix(upstream.Address, "unix:")
		if upstream.Webirc.Enabled {
			if upstream.Webirc.Password == "" {
				config.Upstreams[i].Webirc.Password = "*"
			}
			if upstream.Webirc.Cert != "" {
				cert, err := tls.LoadX509KeyPair(upstream.Webirc.Cert, upstream.Webirc.Key)
				if err != nil {
					return nil, err
				}
				config.Upstreams[i].Webirc.certificates = []tls.Certificate{cert}
			}
		}
	}

	for _, glob := range config.AllowedOrigins {
		globre, err := utils.CompileGlob(glob, false)
		if err != nil {
			return nil, fmt.Errorf("invalid websocket allowed-origin expression: %s", glob)
		}
		config.allowedOriginRegexps = append(config.allowedOriginRegexps, globre)
	}

	config.proxyAllowedFromNets, err = utils.ParseNetList(config.ProxyAllowedFrom)
	if err != nil {
		return nil, fmt.Errorf("Could not parse proxy-allowed-from nets: %v", err.Error())
	}

	return config.postprocessEncodings()
}

func (config *Config) postprocessEncodings() (*Config, error) {
	if config.Transcoding.EnableChardet && len(config.Transcoding.Encodings) != 0 {
		return nil, fmt.Errorf("Cannot enable both chardet and a static list of encodings")
	}

	if config.Transcoding.EnableChardet {
		// from reading the source, this appears to be concurrency-safe:
		config.Transcoding.detector = chardet.NewTextDetector()
	}

	if len(config.Transcoding.Encodings) != 0 {
		for _, encoding := range config.Transcoding.Encodings {
			e, err := ianaindex.IANA.Encoding(encoding)
			if err != nil {
				return nil, fmt.Errorf("Invalid encoding name %s: %v", encoding, err)
			}
			config.Transcoding.encodings = append(config.Transcoding.encodings, e)
		}
	}

	return config, nil
}
