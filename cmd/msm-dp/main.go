package main

import (
	"context"
	"flag"
	"fmt"
	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	logs "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"log"
	"net"
	"net/netip"
	"sync"
)

var (
	port    = flag.Int("port", 9000, "The server port")
	rtpPort = flag.Int("rtpPort", 8050, "rtp port")
)

var wg sync.WaitGroup

var serverIP string
var clientAddrs []netip.AddrPort

// server is used to implement msm_dp.server.
type server struct {
	pb.UnimplementedMsmDataPlaneServer
}

func (s *server) StreamAddDel(_ context.Context, in *pb.StreamData) (*pb.StreamResult, error) {
	log.Printf("Received: message from CP --> Endpoint = %v", in.Endpoint)
	log.Printf("Received: message from CP --> Enable = %v", in.Enable)
	log.Printf("Received: message from CP --> Protocol = %v", in.Protocol)
	log.Printf("Received: message from CP --> Id = %v", in.Id)
	log.Printf("Received: message from CP --> Operation = %v", in.Operation)

	if in.Operation.String() == "CREATE" {
		serverIP = in.Endpoint.Ip
		log.Printf("Server IP: %v", serverIP)
	} else {
		client, err := netip.ParseAddrPort(in.Endpoint.Ip + fmt.Sprintf(":%d", in.Endpoint.Port))
		if err != nil {
			logs.WithError(err).Fatal("unable to create client addr", in.Endpoint.Ip, in.Endpoint.Port)
		}

		if in.Operation.String() == "ADD_EP" {
			clientAddrs = append(clientAddrs, client)
		} else if in.Operation.String() == "DEL_EP" {
			entry := SliceIndex(len(clientAddrs), func(i int) bool { return clientAddrs[i] == client })
			if entry >= 0 {
				clientAddrs = remove(clientAddrs, entry)
				log.Printf("Connection closed, Endpoint Deleted %v", client)
			} else {
				logs.WithError(err).Fatal("unable to find client addr", client)
			}
		}

		log.Printf("Client IPs: %v", clientAddrs)
	}

	return &pb.StreamResult{
		Success: in.Enable,
	}, nil
}

func main() {
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
	go forwardPackets(uint16(*rtpPort))
	go forwardPackets(uint16(*rtpPort + 1))

	log.Printf("Listening for CP messages at %v", lis.Addr())

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

func forwardPackets(port uint16) {
	//Listen to data from server pod
	buffer := make([]byte, 65536)

	udpPort, err := netip.ParseAddrPort(fmt.Sprintf("0.0.0.0:%d", port))

	if err != nil {
		logs.WithError(err).Fatal("unable to create UDP addr:", fmt.Sprintf("0.0.0.0:%d", port))
	}

	sourceConn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(udpPort))

	if err != nil {
		logs.WithError(err).Fatal("Could not listen on address:", serverIP+fmt.Sprintf("0.0.0.0:%d", port))
		return
	}

	defer func(sourceConn net.Conn) {
		err := sourceConn.Close()
		if err != nil {
			logs.WithError(err).Fatal("Could not close sourceConn:", err)
		}
	}(sourceConn)

	logs.Printf("===> Starting proxy, Source at %v", serverIP+fmt.Sprintf(":%d", port))

	for {
		n, err := sourceConn.Read(buffer)

		if err != nil {
			logs.WithError(err).Error("Could not receive a packet")
			continue
		}
		for _, client := range clientAddrs {
			if _, err := sourceConn.WriteToUDPAddrPort(buffer[0:n], client); err != nil {
				logs.WithError(err).Warn("Could not forward packet.")
			}
		}
	}
}

func remove(s []netip.AddrPort, i int) []netip.AddrPort {
	if len(s) > 1 {
		s[i] = s[len(s)-1]
		return s[:len(s)-1]
	}

	log.Printf("deleting only entry in slice")
	return nil
}

func SliceIndex(limit int, predicate func(i int) bool) int {
	for i := 0; i < limit; i++ {
		if predicate(i) {
			return i
		}
	}

	log.Printf("unable to find entry in slice")
	return -1
}
