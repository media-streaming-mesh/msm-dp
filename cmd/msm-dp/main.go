package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sync"

	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

var (
	port    = flag.Int("port", 9000, "The server port")
	rtpPort = flag.Int("rtpPort", 8050, "rtp port")
	logger  *log.Logger
)

var wg sync.WaitGroup

var serverIP string

var clients []Clients

type Clients struct {
	IpAndPort     netip.AddrPort
	StreamType    uint32
	Encapsulation uint32
	Enable        bool
}

// server is used to implement msm_dp.server.
type server struct {
	pb.UnimplementedMsmDataPlaneServer
}

func (s *server) StreamAddDel(_ context.Context, in *pb.StreamData) (*pb.StreamResult, error) {
	logger.Debugf("Received: message from CP --> Endpoint = %v", in.Endpoint)
	logger.Debugf("Received: message from CP --> Enable = %v", in.Enable)
	logger.Debugf("Received: message from CP --> Protocol = %v", in.Protocol)
	logger.Debugf("Received: message from CP --> Id = %v", in.Id)
	logger.Debugf("Received: message from CP --> Operation = %v", in.Operation)

	if in.Operation.String() == "CREATE" {
		serverIP = in.Endpoint.Ip
		logger.Infof("Server IP: %v", serverIP)
	} else {
		client, err := netip.ParseAddrPort(in.Endpoint.Ip + fmt.Sprintf(":%d", in.Endpoint.Port))
		if err != nil {
			logger.WithError(err).Fatal("unable to create client addr", in.Endpoint.Ip, in.Endpoint.Port)
		}

		if in.Operation.String() == "UPD_EP" {
			clients = append(clients, Clients{
				IpAndPort:     client,
				StreamType:    in.Endpoint.QuicStream,
				Encapsulation: in.Endpoint.Encap,
				Enable:        in.Enable,
			})
		} else if in.Operation.String() == "DEL_EP" {
			entry := SliceIndex(len(clients), func(i int) bool { return clients[i].IpAndPort == client })
			if entry >= 0 {
				clients = remove(clients, entry)
				logger.Tracef("Connection closed, Endpoint Deleted %v", client)
			} else {
				logger.WithError(err).Fatal("unable to find client addr", client)
			}
		}

		logger.Infof("Client(s): %+v\n", clients)
	}

	return &pb.StreamResult{
		Success: in.Enable,
	}, nil
}

func init() {
	logger = log.New()

	logger.SetFormatter(&log.TextFormatter{
		ForceColors:     true,
		DisableColors:   false,
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.123",
	})

	setLogLvl(logger)
}

func main() {
	wg.Add(1)
	flag.Parse()

	// open socket to listen to CP messages
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		logger.Fatalf("failed to listen: %v", err)
	}

	// Create gRPC server for messages from CP
	s := grpc.NewServer()
	pb.RegisterMsmDataPlaneServer(s, &server{})

	// Create goroutines for RTP and RTCP
	go forwardPackets(uint16(*rtpPort))
	go forwardPackets(uint16(*rtpPort + 1))

	logger.Info("Listening for CP messages at ", lis.Addr())

	// Serve requests from the control plane
	if err := s.Serve(lis); err != nil {
		logger.Fatalf("failed to serve: %v", err)
	}

	defer func(lis net.Listener) {
		err := lis.Close()
		if err != nil {
			logger.Fatalf("failed to close connection with CP: %v", err)
		}
	}(lis)
}

func forwardPackets(port uint16) {
	//Listen to data from server pod
	buffer := make([]byte, 65536)

	udpPort, err := netip.ParseAddrPort(fmt.Sprintf("0.0.0.0:%d", port))

	if err != nil {
		logger.WithError(err).Fatal("unable to create UDP addr:", fmt.Sprintf("0.0.0.0:%d", port))
	}

	sourceConn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(udpPort))

	if err != nil {
		logger.WithError(err).Fatal("Could not listen on address:", serverIP+fmt.Sprintf("0.0.0.0:%d", port))
		return
	}

	logger.Info("socket is ", sourceConn.LocalAddr().String())

	defer func(sourceConn net.Conn) {
		err := sourceConn.Close()
		if err != nil {
			logger.WithError(err).Fatal("Could not close sourceConn:", err)
		}
	}(sourceConn)

	logger.Info("===> Starting proxy, Source at ", serverIP+fmt.Sprintf(":%d", port))

	for {
		n, err := sourceConn.Read(buffer)

		if err != nil {
			logger.WithError(err).Error("Could not receive a packet")
			continue
		} else {
			logger.Trace("read ", n, " bytes")
		}
		for _, client := range clients {
			if _, err := sourceConn.WriteToUDPAddrPort(buffer[0:n], client.IpAndPort); err != nil {
				logger.WithError(err).Warn("Could not forward packet.")
			} else {
				logger.Trace("sent to ", client.IpAndPort)
			}
		}
	}
}

func remove(s []Clients, i int) []Clients {
	if len(s) > 1 {
		s[i] = s[len(s)-1]
		return s[:len(s)-1]
	}

	logger.Trace("deleting only entry in the list")
	return nil
}

func SliceIndex(limit int, predicate func(i int) bool) int {
	for i := 0; i < limit; i++ {
		if predicate(i) {
			return i
		}
	}

	logger.Trace("unable to find entry in list")
	return -1
}

// sets the log level of the logger
func setLogLvl(l *log.Logger) {
	logLevel := os.Getenv("LOG_LVL")

	switch logLevel {
	case "DEBUG":
		l.SetLevel(log.DebugLevel)
	case "WARN":
		l.SetLevel(log.WarnLevel)
	case "INFO":
		l.SetLevel(log.InfoLevel)
	case "ERROR":
		l.SetLevel(log.ErrorLevel)
	case "TRACE":
		l.SetLevel(log.TraceLevel)
	case "FATAL":
		l.SetLevel(log.FatalLevel)
	default:
		l.SetLevel(log.WarnLevel)
	}
}
