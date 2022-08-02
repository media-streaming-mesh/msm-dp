package proxy

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"time"
)

type trackFlow int

const (
	_TRACK_FLOW_RTP trackFlow = iota
	_TRACK_FLOW_RTCP
)

type track struct {
	rtpPort  int
	rtcpPort int
}

type streamProtocol int

const (
	_STREAM_PROTOCOL_UDP streamProtocol = iota
	_STREAM_PROTOCOL_TCP
)

func (s streamProtocol) String() string {
	if s == _STREAM_PROTOCOL_UDP {
		return "udp"
	}
	return "tcp"
}

type StreamConf struct {
	URL      string `yaml:"url"`
	Protocol string `yaml:"protocol"`
}

type Conf struct {
	ReadTimeout  time.Duration `yaml:"readTimeout"`
	WriteTimeout time.Duration `yaml:"writeTimeout"`
	Server       struct {
		Protocols map[streamProtocol]struct{} `yaml:"protocols"`
		RtspPort  int                         `yaml:"rtspPort"`
		RtpPort   int                         `yaml:"rtpPort"`
		RtcpPort  int                         `yaml:"rtcpPort"`
		ReadUser  string                      `yaml:"readUser"`
		ReadPass  string                      `yaml:"readPass"`
	} `yaml:"server"`
	Streams map[string]StreamConf `yaml:"streams"`
}

// Function which takes in a conf instance and replaces all null with default values
func checkDefaultConf(config *Conf) *Conf {
	if config.ReadTimeout == 0 {
		config.ReadTimeout, _ = time.ParseDuration("5s")
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout, _ = time.ParseDuration("5s")
	}
	if len(config.Streams) == 0 {
		config.Server.Protocols = map[streamProtocol]struct{}{
			_STREAM_PROTOCOL_UDP: {},
			_STREAM_PROTOCOL_TCP: {},
		}
	}

	// Setting default ports
	if config.Server.RtspPort == 0 {
		config.Server.RtspPort = 8554
	}
	if config.Server.RtpPort == 0 {
		config.Server.RtpPort = 8050
	}
	if config.Server.RtcpPort == 0 {
		config.Server.RtcpPort = 8051
	}

	return config
}

func NewConf(
	a_readTimeout string,
	a_writeTimeout string,
	a_protocols []string,
	a_rtspPort int,
	a_rtpPort int,
	a_rtcpPort int,
	a_readUser string,
	a_readPass string,
	a_streams map[string]StreamConf) (*Conf, error) {
	config := checkDefaultConf(&Conf{})
	rto, err := time.ParseDuration(a_readTimeout)
	if err != nil {
		return nil, fmt.Errorf("unable to parse read timeout: %s", err)
	}
	config.ReadTimeout = rto

	wto, err := time.ParseDuration(a_writeTimeout)
	if err != nil {
		return nil, fmt.Errorf("unable to parse write timeout: %s", err)
	}
	config.WriteTimeout = wto

	protocols := make(map[streamProtocol]struct{})
	for _, proto := range a_protocols {
		switch proto {
		case "udp":
			protocols[_STREAM_PROTOCOL_UDP] = struct{}{}

		case "tcp":
			protocols[_STREAM_PROTOCOL_TCP] = struct{}{}

		default:
			return nil, fmt.Errorf("unsupported protocol: '%v'", proto)
		}
	}
	config.Server.Protocols = protocols

	if len(protocols) == 0 {
		return nil, fmt.Errorf("no protocols provided")
	}
	if (a_rtpPort % 2) != 0 {
		return nil, fmt.Errorf("rtp port must be even")
	}
	config.Server.RtpPort = a_rtpPort
	if a_rtcpPort != (a_rtpPort + 1) {
		return nil, fmt.Errorf("rtcp port must be rtp port plus 1")
	}
	config.Server.RtcpPort = a_rtcpPort
	if len(a_streams) == 0 {
		return nil, fmt.Errorf("no streams provided")
	}
	return config, nil
}

// ParseConf parses config from a yml file
func ParseConf(confPath string) (*Conf, error) {
	if confPath == "stdin" {
		var ret Conf
		err := yaml.NewDecoder(os.Stdin).Decode(&ret)
		if err != nil {
			return nil, err
		}
		return &ret, nil
	}
	f, err := os.Open(confPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var ret Conf
	err = yaml.NewDecoder(f).Decode(&ret)
	if err != nil {
		return nil, err
	}

	return checkDefaultConf(&ret), nil
}

// Program contains details about a running proxy
type Program struct {
	conf         Conf
	readTimeout  time.Duration
	writeTimeout time.Duration
	protocols    map[streamProtocol]struct{}
	streams      map[string]*stream
	tcpl         *serverTcpListener
	udplRtp      *serverUdpListener
	udplRtcp     *serverUdpListener
}

// NewProgram creates new program instance which can start or stop a proxy server
func NewProgram(conf *Conf) (*Program, error) {
	p := &Program{
		conf:         *conf,
		readTimeout:  conf.ReadTimeout,
		writeTimeout: conf.WriteTimeout,
		protocols:    conf.Server.Protocols,
		streams:      make(map[string]*stream),
	}

	for path, val := range p.conf.Streams {
		var err error
		p.streams[path], err = newStream(p, path, val)
		if err != nil {
			return nil, fmt.Errorf("error in stream '%s': %s", path, err)
		}
	}

	udplrtp, err := newServerUdpListener(p, p.conf.Server.RtpPort, _TRACK_FLOW_RTP)
	if err != nil {
		return nil, err
	}
	p.udplRtp = udplrtp

	udplRtcp, err := newServerUdpListener(p, p.conf.Server.RtcpPort, _TRACK_FLOW_RTCP)
	if err != nil {
		return nil, err
	}
	p.udplRtcp = udplRtcp

	p.tcpl, err = newServerTcpListener(p)
	if err != nil {
		return nil, err
	}

	return p, nil
}

// Run starts a proxy client
func (p *Program) Run() {
	for _, s := range p.streams {
		go s.run()
	}

	go p.udplRtp.run()
	go p.udplRtcp.run()
	go p.tcpl.run()

	select {}
}

// Stop closes a proxy client
func (p *Program) Stop() {
	for _, s := range p.streams {
		s.close()
	}

	p.tcpl.close()
	p.udplRtcp.close()
	p.udplRtp.close()
}
