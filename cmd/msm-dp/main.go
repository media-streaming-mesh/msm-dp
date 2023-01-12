package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"sync"

	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

var (
	port    = flag.Int("port", 9000, "The server port")
	rtpPort = flag.Int("rtpPort", 8050, "rtp port")
)

var wg sync.WaitGroup

var serverIP string

var clients []Clients

type Clients struct {
	IpAndPort     netip.AddrPort
	rtcp          netip.AddrPort
	StreamType    uint32
	Encapsulation uint32
	Enable        bool
}

// server is used to implement msm_dp.server.
type server struct {
	pb.UnimplementedMsmDataPlaneServer
}

func (s *server) StreamAddDel(_ context.Context, in *pb.StreamData) (*pb.StreamResult, error) {
	//log.Debugf("Received: message from CP --> Endpoint = %v", in.Endpoint)
	//log.Debugf("Received: message from CP --> Enable = %v", in.Enable)
	//log.Debugf("Received: message from CP --> Protocol = %v", in.Protocol)
	//log.Debugf("Received: message from CP --> Id = %v", in.Id)

	log.Debugf("Received: message from CP --> Operation = %v", in.Operation)

	switch in.Operation.String() {
	case "CREATE":
		serverIP = in.Endpoint.Ip
		log.Infof("Server IP: %v", serverIP)

	case "UPDATE":
		log.Errorf("unexpected UPDATE")

	case "DELETE":
		clients = nil
		log.Debugf("All clients are deleted %v", clients)

	default:
		client, err := netip.ParseAddrPort(in.Endpoint.Ip + fmt.Sprintf(":%d", in.Endpoint.Port))
		if err != nil {
			log.WithError(err).Fatal("unable to create client addr", in.Endpoint.Ip, in.Endpoint.Port)
		}
		rtcp, err := netip.ParseAddrPort(in.Endpoint.Ip + fmt.Sprintf(":%d", in.Endpoint.Port+1))
		if err != nil {
			log.WithError(err).Fatal("unable to create client addr", in.Endpoint.Ip, in.Endpoint.Port+1)
		}
		if in.Operation.String() == "ADD_EP" {
			clients = append(clients, Clients{
				IpAndPort:     client,
				rtcp:          rtcp,
				StreamType:    in.Endpoint.QuicStream,
				Encapsulation: in.Endpoint.Encap,
				Enable:        in.Enable,
			})
			log.Debugf("Client Added with IP %v", client)
			log.Debugf("Client stream data enable state is %v", in.Enable)
		} else {
			entry := SliceIndex(len(clients), func(i int) bool { return clients[i].IpAndPort == client })
			if entry >= 0 {
				if in.Operation.String() == "UPD_EP" {
					clients[entry].Enable = in.Enable
				} else if in.Operation.String() == "DEL_EP" {
					clients = remove(clients, entry)
					log.Debugf("Connection closed, Endpoint Deleted %v", client)
				} else {
					log.WithError(err).Fatal("Invalid operation", in.Operation.String())
				}
			} else {
				log.WithError(err).Fatal("Unable to find client addr", client)
			}
		}
	}

	log.Infof("Client(s): %+v\n", clients)

	return &pb.StreamResult{
		Success: in.Enable,
	}, nil
}

func main() {
	log.SetFormatter(&log.TextFormatter{
		ForceColors:     true,
		DisableColors:   false,
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	log.SetLevel(log.DebugLevel)
	// log.SetReportCaller(true)

	wg.Add(1)
	flag.Parse()

	// open socket to listen to CP messages
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Create gRPC server for messages from CP
	s := grpc.NewServer()
	pb.RegisterMsmDataPlaneServer(s, &server{})

	// Create goroutines for RTP and RTCP
	go forwardRTPPackets(uint16(*rtpPort))
	go forwardRTCPPackets(uint16(*rtpPort + 1))

	log.Info("Listening for CP messages at ", lis.Addr())

	// Serve requests from the control plane
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}

	defer func(lis net.Listener) {
		err := lis.Close()
		if err != nil {
			log.Fatalf("failed to close connection with CP: %v", err)
		}
	}(lis)
}

func forwardRTPPackets(port uint16) {
	//Listen to data from server pod
	buffer := make([]byte, 65536)

	udpPort, err := netip.ParseAddrPort(fmt.Sprintf("0.0.0.0:%d", port))

	if err != nil {
		log.WithError(err).Fatal("unable to create UDP addr:", fmt.Sprintf("0.0.0.0:%d", port))
	}

	sourceConn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(udpPort))

	if err != nil {
		log.WithError(err).Fatal("Could not listen on address:", serverIP+fmt.Sprintf("0.0.0.0:%d", port))
		return
	}

	log.Info("socket is ", sourceConn.LocalAddr().String())

	defer func(sourceConn net.Conn) {
		err := sourceConn.Close()
		if err != nil {
			log.WithError(err).Fatal("Could not close sourceConn:", err)
		}
	}(sourceConn)

	log.Info("===> Starting proxy, Source at ", serverIP+fmt.Sprintf(":%d", port))

	for {
		n, err := sourceConn.Read(buffer)

		if err != nil {
			log.WithError(err).Error("Could not receive a packet")
			continue
		} else {
			log.Trace("read ", n, " bytes")
		}
		for _, client := range clients {
			if _, err := sourceConn.WriteToUDPAddrPort(buffer[0:n], client.IpAndPort); err != nil {
				log.WithError(err).Warn("Could not forward packet.")
			} else {
				log.Trace("sent to ", client.IpAndPort)
			}
		}
	}
}

func forwardRTCPPackets(port uint16) {
	//Listen to data from server pod
	buffer := make([]byte, 65536)

	udpPort, err := netip.ParseAddrPort(fmt.Sprintf("0.0.0.0:%d", port))

	if err != nil {
		log.WithError(err).Fatal("unable to create UDP addr:", fmt.Sprintf("0.0.0.0:%d", port))
	}

	sourceConn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(udpPort))

	if err != nil {
		log.WithError(err).Fatal("Could not listen on address:", serverIP+fmt.Sprintf("0.0.0.0:%d", port))
		return
	}

	log.Info("socket is ", sourceConn.LocalAddr().String())

	defer func(sourceConn net.Conn) {
		err := sourceConn.Close()
		if err != nil {
			log.WithError(err).Fatal("Could not close sourceConn:", err)
		}
	}(sourceConn)

	log.Info("===> Starting proxy, Source at ", serverIP+fmt.Sprintf(":%d", port))

	for {
		n, err := sourceConn.Read(buffer)

		if err != nil {
			log.WithError(err).Error("Could not receive rtcp packet")
			continue
		} else {
			log.Trace("read ", n, " bytes")
		}
		for _, client := range clients {
			if _, err := sourceConn.WriteToUDPAddrPort(buffer[0:n], client.rtcp); err != nil {
				log.WithError(err).Warn("Could not forward rtcp packet.")
			} else {
				log.Trace("sent to ", client.rtcp)
			}
		}
	}
}

func remove(s []Clients, i int) []Clients {
	if len(s) > 1 {
		s[i] = s[len(s)-1]
		return s[:len(s)-1]
	}

	log.Trace("deleting only entry in the list")
	return nil
}

func SliceIndex(limit int, predicate func(i int) bool) int {
	for i := 0; i < limit; i++ {
		if predicate(i) {
			return i
		}
	}

	log.Trace("unable to find entry in list")
	return -1
}
