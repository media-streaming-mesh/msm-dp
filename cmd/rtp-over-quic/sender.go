package rtp_over_quic

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"math"
	"net"
	"time"

	"github.com/lucas-clemente/quic-go"
	quiclogging "github.com/lucas-clemente/quic-go/logging"
	"github.com/lucas-clemente/quic-go/quicvarint"
	"github.com/mengelbart/rtp-over-quic/cc"
	"github.com/mengelbart/rtp-over-quic/logging"
	"github.com/mengelbart/rtp-over-quic/rtp"
	"github.com/pion/interceptor"
	pionrtp "github.com/pion/rtp"
)

const rtpOverQUICALPN = "rtp-mux-rtp-over-quic"

type SenderOption func(*SenderConfig) error

func RemoteAddress(addr string) SenderOption {
	return func(sc *SenderConfig) error {
		sc.remoteAddr = addr
		return nil
	}
}

func SetSenderQLOGDirName(dir string) SenderOption {
	return func(sc *SenderConfig) error {
		sc.qlogDirectoryName = dir
		return nil
	}
}

func SetSenderSSLKeyLogFileName(file string) SenderOption {
	return func(sc *SenderConfig) error {
		sc.sslKeyLogFileName = file
		return nil
	}
}

func SetSenderQUICCongestionControlAlgorithm(algorithm cc.Algorithm) SenderOption {
	return func(sc *SenderConfig) error {
		sc.cc = algorithm
		return nil
	}
}

func SetLocalRFC8888(enabled bool) SenderOption {
	return func(sc *SenderConfig) error {
		sc.localRFC8888 = enabled
		return nil
	}
}

func SetTransportMode(mode TransportMode) SenderOption {
	return func(sc *SenderConfig) error {
		sc.transportMode = mode
		return nil
	}
}

type SenderConfig struct {
	remoteAddr        string
	qlogDirectoryName string
	sslKeyLogFileName string

	cc            cc.Algorithm
	localRFC8888  bool
	maxMTU        uint
	transportMode TransportMode
}

type Sender struct {
	*SenderConfig

	conn                quic.Connection
	metricsTracer       *RTTTracer
	interceptorRegistry *interceptor.Registry
	interceptor         interceptor.Interceptor
	localFeedback       *localRFC8888Generator

	flowIDs map[uint64]struct{}
}

func NewSender(r *interceptor.Registry, opts ...SenderOption) (*Sender, error) {
	s := &Sender{
		SenderConfig: &SenderConfig{
			remoteAddr:        ":4242",
			qlogDirectoryName: "",
			sslKeyLogFileName: "",
			cc:                cc.Reno,
			localRFC8888:      false,
			maxMTU:            1300,
			transportMode:     ANY,
		},
		conn:                nil,
		metricsTracer:       nil,
		interceptorRegistry: r,
		localFeedback:       nil,
		flowIDs:             make(map[uint64]struct{}),
	}
	for _, opt := range opts {
		if err := opt(s.SenderConfig); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Sender) newFlowID() (uint64, error) {
	for i := uint64(0); i < math.MaxUint64; i++ {
		if _, ok := s.flowIDs[i]; !ok {
			s.flowIDs[i] = struct{}{}
			return i, nil
		}
	}
	return 0, errors.New("too many flows, no unused IDs left")
}

func (s *Sender) Connect(ctx context.Context) error {
	qlogWriter, err := logging.GetQLOGTracer(s.qlogDirectoryName)
	if err != nil {
		return err
	}
	keyLogger, err := logging.GetKeyLogger(s.sslKeyLogFileName)
	if err != nil {
		return err
	}
	tlsConf := &tls.Config{
		KeyLogWriter:       keyLogger,
		InsecureSkipVerify: true,
		NextProtos:         []string{rtpOverQUICALPN},
	}
	s.metricsTracer = NewTracer()
	tracers := []quiclogging.Tracer{s.metricsTracer}
	if qlogWriter != nil {
		tracers = append(tracers, qlogWriter)
	}
	tracer := quiclogging.NewMultiplexedTracer(tracers...)
	quicConf := &quic.Config{
		EnableDatagrams:       true,
		HandshakeIdleTimeout:  15 * time.Second,
		Tracer:                tracer,
		DisableCC:             s.cc != cc.Reno,
		MaxIncomingStreams:    1 << 60,
		MaxIncomingUniStreams: 1 << 60,
	}
	conn, err := quic.DialAddrContext(ctx, s.remoteAddr, tlsConf, quicConf)
	if err != nil {
		return err
	}
	s.conn = conn

	i, err := s.interceptorRegistry.Build("")
	if err != nil {
		return err
	}
	s.interceptor = i

	rtcpReader := s.interceptor.BindRTCPReader(interceptor.RTCPReaderFunc(
		func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
			return len(b), a, nil
		}),
	)

	rtcpChan := make(chan rtp.RTCPFeedback)
	go rtp.ReadRTCP(ctx, rtcpReader, rtcpChan)
	go s.readFromNetwork(ctx, rtcpChan)

	if s.localRFC8888 {
		s.localFeedback = newLocalRFC8888Generator(0, s.metricsTracer, func(r rtp.RTCPFeedback) {
			rtcpChan <- r
		})
		go s.localFeedback.run(ctx)
	}

	return nil
}

func (s *Sender) readFromNetwork(ctx context.Context, rtcpChan chan rtp.RTCPFeedback) {
	for {
		buf, err := s.conn.ReceiveMessage()
		if err != nil {
			if e, ok := err.(*quic.ApplicationError); ok && e.ErrorCode == 0 {
				log.Printf("QUIC received application error, exiting reader routine: %v", err)
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				log.Printf("QUIC connection timed out, exiting datagram receiver routine: %v", err)
				return
			}
			log.Printf("failed to receive QUIC datagram: %v", err)
			continue
		}
		// TODO: If multiple RTCP flows are required, demultiplex on id here
		id, err := quicvarint.Read(bytes.NewReader(buf))
		if err != nil {
			log.Printf("failed to read flow ID: %v, dropping datagram", err)
			continue
		}
		rtcpChan <- rtp.RTCPFeedback{
			Buffer:     buf[quicvarint.Len(id):],
			Attributes: nil,
		}
	}
}

func (s *Sender) writeDgram(buf []byte, cb func(bool)) (int, error) {
	return len(buf), s.conn.SendMessage(buf, cb)
}

func (s *Sender) writeStream(buf []byte) (int, error) {
	stream, err := s.conn.OpenUniStreamSync(context.Background())
	if err != nil {
		return 0, err
	}
	defer stream.Close()
	return stream.Write(buf)
}

func (s *Sender) NewMediaStreamWithFlowID(id uint64) interceptor.RTPWriter {
	var idBuffer bytes.Buffer
	idWriter := quicvarint.NewWriter(&idBuffer)
	quicvarint.Write(idWriter, id)
	idBytes := idBuffer.Bytes()
	return s.interceptor.BindLocalStream(&interceptor.StreamInfo{}, interceptor.RTPWriterFunc(
		func(header *pionrtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
			headerBuf, err := header.Marshal()
			if err != nil {
				return 0, err
			}
			pl := append(idBytes, headerBuf...)
			pl = append(pl, payload...)

			if s.transportMode == DGRAM {
				// log.Printf("send dgram with ACK callback due to DGRAM transportMode")
				return s.writeDgram(pl, s.ackCallback(time.Now(), header.SSRC, header.MarshalSize()+len(pl), header.SequenceNumber))
			}

			if s.transportMode == STREAM {
				// log.Printf("send stream due to STREAM transportMode")
				return s.writeStream(pl)
			}

			mtu := uint(len(pl))
			if mtu > s.maxMTU {
				if s.localRFC8888 {
					log.Println("WARNING: Sending on stream due to too large MTU, but local CC FB (RFC8888) generation was requested, which is currently not implemented for QUIC streams")
				}
				// log.Printf("send stream due to mtu>s.maxMTU")
				return s.writeStream(pl)
			}

			if attributes == nil {
				// log.Printf("send dgram with ACK callback due to nil attributes")
				return s.writeDgram(pl, s.ackCallback(time.Now(), header.SSRC, header.MarshalSize()+len(pl), header.SequenceNumber))
			}

			reliability := attributes.Get(rtp.RELIABILITY)
			if reliability != nil && reliability.(rtp.Reliability) == rtp.REQUIRED {
				// log.Printf("send stream due reliability == REQUIRED")
				return s.writeStream(pl)
			}
			// log.Printf("send dgram due reliability != REQUIRED")
			return s.writeDgram(pl, s.ackCallback(time.Now(), header.SSRC, header.MarshalSize()+len(pl), header.SequenceNumber))
		},
	))
}

func (s *Sender) NewMediaStream() (interceptor.RTPWriter, error) {
	id, err := s.newFlowID()
	if err != nil {
		return nil, err
	}
	return s.NewMediaStreamWithFlowID(id), nil
}

func (s *Sender) ackCallback(sent time.Time, ssrc uint32, size int, seqNr uint16) func(bool) {
	if s.localRFC8888 {
		return func(b bool) {
			if b {
				s.localFeedback.ack(ackedPkt{
					sentTS: sent,
					ssrc:   ssrc,
					size:   size,
					seqNr:  seqNr,
				})
			}
		}
	}
	return nil
}

type DataStreamWriter struct {
	io.Writer
}

func (s *Sender) NewDataStreamWithFlowID(ctx context.Context, id uint64) (io.Writer, error) {
	stream, err := s.conn.OpenUniStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	var idBuffer bytes.Buffer
	idWriter := quicvarint.NewWriter(&idBuffer)
	quicvarint.Write(idWriter, id)
	idBytes := idBuffer.Bytes()
	_, err = stream.Write(idBytes)
	if err != nil {
		return nil, err
	}
	return &DataStreamWriter{
		Writer: stream,
	}, nil
}

func (s *Sender) NewDataStreamWithoutFlowID(ctx context.Context) (io.Writer, error) {
	stream, err := s.conn.OpenUniStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &DataStreamWriter{
		Writer: stream,
	}, nil
}

func (s *Sender) NewDataStreamWithDefaultFlowID(ctx context.Context) (io.Writer, error) {
	id, err := s.newFlowID()
	if err != nil {
		return nil, err
	}
	return s.NewDataStreamWithFlowID(ctx, id)
}
