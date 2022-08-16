package proxy

import (
	"fmt"
	"gopkg.in/alecthomas/kingpin.v2"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
)

var Version string = "v0.0.0"

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

type args struct {
	version      bool
	protocolsStr string
	rtspPort     int
	rtpPort      int
	rtcpPort     int
	readTimeout  time.Duration
	writeTimeout time.Duration
	publishUser  string
	publishPass  string
	readUser     string
	readPass     string
	preScript    string
	postScript   string
}

type program struct {
	args      args
	protocols map[streamProtocol]struct{}
	tcpl      *serverTcpListener
	udplRtp   *serverUdpListener
	udplRtcp  *serverUdpListener
}

func newProgram(sargs []string) (*program, error) {
	kingpin.CommandLine.Help = "MSM-Proxy " + Version + "\n\n" +
		"RTP Proxy."

	argVersion := kingpin.Flag("version", "print version").Bool()
	argProtocolsStr := kingpin.Flag("protocols", "supported protocols").Default("udp,tcp").String()
	argRtspPort := kingpin.Flag("rtsp-port", "port of the RTSP TCP listener").Default("8554").Int()
	argRtpPort := kingpin.Flag("rtp-port", "port of the RTP UDP listener").Default("8050").Int()
	argRtcpPort := kingpin.Flag("rtcp-port", "port of the RTCP UDP listener").Default("8051").Int()
	argReadTimeout := kingpin.Flag("read-timeout", "timeout of read operations").Default("5s").Duration()
	argWriteTimeout := kingpin.Flag("write-timeout", "timeout of write operations").Default("5s").Duration()
	argPublishUser := kingpin.Flag("publish-user", "optional username required to publish").Default("").String()
	argPublishPass := kingpin.Flag("publish-pass", "optional password required to publish").Default("").String()
	argReadUser := kingpin.Flag("read-user", "optional username required to read").Default("").String()
	argReadPass := kingpin.Flag("read-pass", "optional password required to read").Default("").String()
	argPreScript := kingpin.Flag("pre-script", "optional script to run on client connect").Default("").String()
	argPostScript := kingpin.Flag("post-script", "optional script to run on client disconnect").Default("").String()

	kingpin.MustParse(kingpin.CommandLine.Parse(sargs))

	args := args{
		version:      *argVersion,
		protocolsStr: *argProtocolsStr,
		rtspPort:     *argRtspPort,
		rtpPort:      *argRtpPort,
		rtcpPort:     *argRtcpPort,
		readTimeout:  *argReadTimeout,
		writeTimeout: *argWriteTimeout,
		publishUser:  *argPublishUser,
		publishPass:  *argPublishPass,
		readUser:     *argReadUser,
		readPass:     *argReadPass,
		preScript:    *argPreScript,
		postScript:   *argPostScript,
	}

	if args.version == true {
		log.Println(Version)
		os.Exit(0)
	}

	protocols := make(map[streamProtocol]struct{})
	for _, proto := range strings.Split(args.protocolsStr, ",") {
		switch proto {
		case "udp":
			protocols[_STREAM_PROTOCOL_UDP] = struct{}{}

		case "tcp":
			protocols[_STREAM_PROTOCOL_TCP] = struct{}{}

		default:
			return nil, fmt.Errorf("unsupported protocol: %s", proto)
		}
	}
	if len(protocols) == 0 {
		return nil, fmt.Errorf("no protocols provided")
	}
	if (args.rtpPort % 2) != 0 {
		return nil, fmt.Errorf("rtp port must be even")
	}
	if args.rtcpPort != (args.rtpPort + 1) {
		return nil, fmt.Errorf("rtcp and rtp ports must be consecutive")
	}
	if args.publishUser != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(args.publishUser) {
			return nil, fmt.Errorf("publish username must be alphanumeric")
		}
	}
	if args.publishPass != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(args.publishPass) {
			return nil, fmt.Errorf("publish password must be alphanumeric")
		}
	}
	if args.readUser != "" && args.readPass == "" || args.readUser == "" && args.readPass != "" {
		return nil, fmt.Errorf("read username and password must be both filled")
	}
	if args.readUser != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(args.readUser) {
			return nil, fmt.Errorf("read username must be alphanumeric")
		}
	}
	if args.readPass != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(args.readPass) {
			return nil, fmt.Errorf("read password must be alphanumeric")
		}
	}
	if args.readUser != "" && args.readPass == "" || args.readUser == "" && args.readPass != "" {
		return nil, fmt.Errorf("read username and password must be both filled")
	}

	log.Printf("MSM-Proxy %s", Version)

	p := &program{
		args:      args,
		protocols: protocols,
	}

	var err error

	p.udplRtp, err = newServerUdpListener(p, args.rtpPort, _TRACK_FLOW_RTP)
	if err != nil {
		return nil, err
	}

	p.udplRtcp, err = newServerUdpListener(p, args.rtcpPort, _TRACK_FLOW_RTCP)
	if err != nil {
		return nil, err
	}

	p.tcpl, err = newServerTcpListener(p)
	if err != nil {
		return nil, err
	}

	go p.udplRtp.run()
	go p.udplRtcp.run()
	go p.tcpl.run()

	return p, nil
}

func (p *program) close() {
	log.Println("Closing all Listeners---> close()")
	p.tcpl.close()
	p.udplRtcp.close()
	p.udplRtp.close()
}

func Run() {
	_, err := newProgram(os.Args[1:])
	if err != nil {
		log.Fatal("ERR: ", err)
	}
	select {}
}
