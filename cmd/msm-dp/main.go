package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"

	"google.golang.org/grpc/health/grpc_health_v1"

	"google.golang.org/grpc"

	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	log "github.com/sirupsen/logrus"
)

var (
	port    = flag.Int("port", 9000, "The server port")
	rtpPort = flag.Int("rtpPort", 8050, "rtp port")
)

type Endpoint struct {
	enabled bool
	address net.UDPAddr
}

type Stream struct {
	server  net.UDPAddr
	clients map[string]Endpoint
}

var (
	streams   = make(map[uint32]Stream)
	streamMap = make(map[string]uint32)
)

// server is used to implement msm_dp.server
type server struct {
	pb.UnimplementedMsmDataPlaneServer
}

func (s *server) StreamAddDel(_ context.Context, in *pb.StreamData) (*pb.StreamResult, error) {
	switch in.Operation.String() {
	case "CREATE":
		// check if the stream already exists in the streams map
		_, exists := streams[in.Id]
		if !exists {
			streams[in.Id] = Stream{
				server:  net.UDPAddr{IP: net.ParseIP(in.Endpoint.Ip), Port: int(in.Endpoint.Port), Zone: ""},
				clients: make(map[string]Endpoint),
			}
			log.Infof("New stream ID: %v, source %v:%v", in.Id, in.Endpoint.Ip, in.Endpoint.Port)
			streamMap[fmt.Sprintf("%s:%d", in.Endpoint.Ip, in.Endpoint.Port)] = in.Id
		} else {
			log.Errorf("Stream with ID %d already exists", in.Id)
		}
	case "UPDATE":
		log.Errorf("unexpected UPDATE")
	case "DELETE":
		delete(streams, in.Id)
		log.Infof("Deleted stream ID: %v", in.Id)
	default:
		client := net.UDPAddr{IP: net.ParseIP(in.Endpoint.Ip), Port: int(in.Endpoint.Port), Zone: ""}
		stream, ok := streams[in.Id]
		if !ok {
			log.Errorf("Stream with ID %d doesn't exists", in.Id)
			return &pb.StreamResult{}, nil
		}
		if in.Operation.String() == "ADD_EP" {
			stream.clients[client.String()] = Endpoint{enabled: in.Enable, address: client}
			streams[in.Id] = stream
			log.Infof("Client %v added to stream %v", client, in.Id)
		} else if in.Operation.String() == "UPD_EP" {
			endpoint, ok := stream.clients[client.String()]
			if !ok {
				log.Errorf("Endpoint %v doesn't exist in the stream %v", client, in.Id)
				return &pb.StreamResult{}, nil
			}
			endpoint.enabled = in.Enable
			stream.clients[client.String()] = endpoint
			streams[in.Id] = stream
			log.Infof("Client %v updated in stream %v", client, in.Id)
		} else if in.Operation.String() == "DEL_EP" {
			_, ok := stream.clients[client.String()]
			if !ok {
				log.Errorf("Endpoint %v doesn't exist in the stream %v", client, in.Id)
				return &pb.StreamResult{}, nil
			}
			delete(stream.clients, client.String())
			streams[in.Id] = stream
			log.Infof("Client %v deleted from stream %v", client, in.Id)
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

		streamID, ok := streamMap[fmt.Sprintf("%s:%d", sourceAddr.IP.String(), sourceAddr.Port)]
		if !ok {
			log.Tracef("RTP stream for server %s:%d not found", sourceAddr.IP.String(), sourceAddr.Port)
			continue
		}
		stream, ok := streams[streamID]
		if !ok {
			log.Errorf("stream %v doesn't exists", streamID)
			continue
		}

		if !sourceAddr.IP.Equal(stream.server.IP) || sourceAddr.Port != stream.server.Port {
			log.Errorf("RTP packet received from unknown server %v, expected %v", sourceAddr, stream.server)
			continue
		}

		for _, endpoint := range stream.clients {
			if endpoint.enabled {
				if _, err := sourceConn.WriteToUDP(buffer[0:n], &endpoint.address); err != nil {
					log.WithError(err).Warn("Could not forward RTP packet.")
				} else {
					log.Tracef("RTP packet sent to %v", endpoint.address)
				}
			}
		}
	}
}

func forwardRTCPPackets(port uint16) {
	sourceAddr := &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: int(port), Zone: ""}
	sourceConn, err := net.ListenUDP("udp", sourceAddr)
	if err != nil {
		log.WithError(err).Fatal("Could not start listening on RTCP port.")
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
			log.WithError(err).Warn("Error while reading RTCP packet.")
			continue
		}

		streamID, ok := streamMap[fmt.Sprintf("%s:%d", sourceAddr.IP.String(), sourceAddr.Port-1)]
		if !ok {
			log.Tracef("RTCP stream for server %s:%d not found", sourceAddr.IP.String(), sourceAddr.Port)
			continue
		}
		stream, ok := streams[streamID]
		if !ok {
			log.Errorf("stream %v doesn't exists", streamID)
			continue
		}

		if !sourceAddr.IP.Equal(stream.server.IP) || sourceAddr.Port != stream.server.Port+1 {
			log.Errorf("RTCP packet received from unknown server %v, expected %v", sourceAddr, stream.server)
			continue
		}

		for _, endpoint := range stream.clients {
			if endpoint.enabled {
				RTCPAddress := net.UDPAddr{IP: endpoint.address.IP, Port: endpoint.address.Port + 1, Zone: endpoint.address.Zone}
				if _, err := sourceConn.WriteToUDP(buffer[0:n], &RTCPAddress); err != nil {
					log.WithError(err).Warn("Could not forward RTCP packet.")
				} else {
					log.Tracef("RTP packet sent to %v", endpoint.address)
				}
			}
		}
	}
}

func main() {
	log.SetOutput(os.Stdout)
	logFormat := os.Getenv("LOG_TYPE")
	switch strings.ToLower(logFormat) {
	case "json":
		log.SetFormatter(&log.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
			PrettyPrint:     true,
		})
	default:
		log.SetFormatter(&log.TextFormatter{
			ForceColors:     true,
			DisableColors:   false,
			FullTimestamp:   true,
			TimestampFormat: "01-02-2006 15:04:05",
		})
	}

	logLevel := os.Getenv("LOG_LEVEL")
	switch strings.ToLower(logLevel) {
	case "trace":
		log.SetLevel(log.TraceLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.SetLevel(log.DebugLevel)
	}

	flag.Parse()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterMsmDataPlaneServer(s, &server{})

	healthService := NewHealthChecker()
	grpc_health_v1.RegisterHealthServer(s, healthService)

	go forwardRTPPackets(uint16(*rtpPort))
	go forwardRTCPPackets(uint16(*rtpPort + 1))

	log.Info("Listening for CP messages at ", lis.Addr())

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
		log.Fatalf("failed to serve: %v", err)
	}

	defer func(lis net.Listener) {
		err := lis.Close()
		if err != nil {
			log.Fatalf("failed to close connection with CP: %v", err)
		}
	}(lis)
}
