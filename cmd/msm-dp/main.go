package main

import (
	"context"
	"flag"
	"fmt"
	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"net"
)

var (
	port    = flag.Int("port", 9000, "The server port")
	rtpPort = flag.Int("rtpPort", 8050, "rtp port")
)

var streams []Streams

type Server struct {
	Addr *net.UDPAddr
	ID   uint32
}

type Streams struct {
	server  *Server
	clients []Client
}
type Client struct {
	Addr *net.UDPAddr
	ID   uint32
}

// server is used to implement msm_dp.server.
type server struct {
	pb.UnimplementedMsmDataPlaneServer
}

func (s *server) StreamAddDel(_ context.Context, in *pb.StreamData) (*pb.StreamResult, error) {
	log.Debugf("Received: message from CP --> Operation = %v", in.Operation)
	switch in.Operation.String() {
	case "CREATE":
		server := Server{
			Addr: &net.UDPAddr{IP: net.ParseIP(in.Endpoint.Ip), Port: int(in.Endpoint.Port), Zone: ""},
			ID:   in.Id,
		}
		stream := Streams{
			server: &server,
		}
		// check if the stream already exists in the streams array
		exists := false
		for _, s := range streams {
			if s.server.ID == in.Id {
				exists = true
				break
			}
		}
		if !exists {
			streams = append(streams, stream)
			log.Infof("Server IP: %v", server.Addr)
		} else {
			log.Errorf("Stream with ID %d already exists", in.Id)
		}
	case "UPDATE":
		log.Errorf("unexpected UPDATE")
	case "DELETE":
		for i, stream := range streams {
			if stream.server.ID == in.Id {
				if len(stream.clients) == 0 {
					// remove the element from the array
					copy(streams[i:], streams[i+1:])
					streams[len(streams)-1] = Streams{}
					streams = streams[:len(streams)-1]
					log.Debugf("All clients and server are deleted %v", streams)
					break
				}
			}
		}
	default:
		client := &net.UDPAddr{IP: net.ParseIP(in.Endpoint.Ip), Port: int(in.Endpoint.Port), Zone: ""}
		if in.Operation.String() == "ADD_EP" {
			// Iterate through the streams
			for _, stream := range streams {
				// check if the id is match
				if stream.server.ID == in.Id {
					stream.clients = append(stream.clients, Client{
						Addr: client,
						ID:   stream.server.ID,
					})
					log.Infof("Client IP: %v added to server ID: %v", client, in.Id)
					break
				}
			}
		}

		if in.Operation.String() == "DEL_EP" {
			// Iterate through the streams
			for _, stream := range streams {
				// check if the id is match
				if stream.server.ID == in.Id {
					for i, c := range stream.clients {
						if c.Addr.String() == client.String() {
							stream.clients = append(stream.clients[:i], stream.clients[i+1:]...)
							log.Infof("Client IP: %v deleted from server ID: %v", client, in.Id)
							break
						}
					}
					break
				}
			}
		}
	}
	return &pb.StreamResult{}, nil
}

func forwardRTPPackets(port uint16) {
	sourceAddr := &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: int(port), Zone: ""}
	sourceConn, err := net.ListenUDP("udp", sourceAddr)
	if err != nil {
		log.WithError(err).Fatal("Could not start listening on RTP port.")
	}
	defer func(sourceConn *net.UDPConn) {
		err := sourceConn.Close()
		if err != nil {
			log.WithError(err).Warn("Unable to close sourceConn")
		}
	}(sourceConn)

	buffer := make([]byte, 65507)
	for {
		n, sourceAddr, err := sourceConn.ReadFromUDP(buffer)
		if err != nil {
			log.WithError(err).Warn("Error while reading RTP packet.")
			continue
		}

		for _, stream := range streams {
			if sourceAddr.IP.Equal(stream.server.Addr.IP) {
				for _, client := range stream.clients {
					if _, err := sourceConn.WriteToUDP(buffer[0:n], client.Addr); err != nil {
						log.WithError(err).Warn("Could not forward packet.")
					} else {
						log.Trace("sent to ", client.Addr)
					}
				}
			}
		}
	}
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
	//go forwardRTCPPackets(uint16(*rtpPort + 1))

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
