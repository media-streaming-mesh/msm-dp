package rtp_over_quic

import (
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/mengelbart/rtp-over-quic/cc"
	"github.com/mengelbart/rtp-over-quic/logging"
)

type TransportMode int

func TransportModeFromString(s string) TransportMode {
	switch s {
	case "rtp-over-quic-dgram":
		return DGRAM
	case "rtp-over-quic-stream":
		return STREAM
	default:
		return ANY
	}
}

const (
	ANY TransportMode = iota
	DGRAM
	STREAM
)

func listen(
	addr string,
	ccAlgo cc.Algorithm,
	qlogDirectoryName string,
	sslKeyLogFileName string,
) (quic.Listener, error) {
	qlogWriter, err := logging.GetQLOGTracer(qlogDirectoryName)
	if err != nil {
		return nil, err
	}
	keyLogger, err := logging.GetKeyLogger(sslKeyLogFileName)
	if err != nil {
		return nil, err
	}
	quicConf := &quic.Config{
		EnableDatagrams:       true,
		HandshakeIdleTimeout:  15 * time.Second,
		Tracer:                qlogWriter,
		DisableCC:             ccAlgo != cc.Reno,
		MaxIncomingStreams:    1 << 60,
		MaxIncomingUniStreams: 1 << 60,
	}
	tlsConf := generateTLSConfig(keyLogger)
	return quic.ListenAddr(addr, tlsConf, quicConf)
}
