package main

import (
	"context"
	"flag"
	"fmt"
	"net"

	"github.com/media-streaming-mesh/msm-dp/cmd/util"

	"google.golang.org/grpc/health/grpc_health_v1"

	"google.golang.org/grpc"

	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
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
			util.Infof("New stream ID: %v, source %v:%v", in.Id, in.Endpoint.Ip, in.Endpoint.Port)
			streamMap[fmt.Sprintf("%s:%d", in.Endpoint.Ip, in.Endpoint.Port)] = in.Id
		} else {
			util.Errorf("Stream with ID %d already exists", in.Id)
		}
	case "UPDATE":
		util.Errorf("unexpected UPDATE")
	case "DELETE":
		delete(streams, in.Id)
		util.Infof("Deleted stream ID: %v", in.Id)
	default:
		client := net.UDPAddr{IP: net.ParseIP(in.Endpoint.Ip), Port: int(in.Endpoint.Port), Zone: ""}
		stream, ok := streams[in.Id]
		if !ok {
			util.Errorf("Stream with ID %d doesn't exists", in.Id)
			return &pb.StreamResult{}, nil
		}
		if in.Operation.String() == "ADD_EP" {
			stream.clients[client.String()] = Endpoint{enabled: in.Enable, address: client}
			streams[in.Id] = stream
			util.Infof("Client %v added to stream %v", client, in.Id)
		} else if in.Operation.String() == "UPD_EP" {
			endpoint, ok := stream.clients[client.String()]
			if !ok {
				util.Errorf("Endpoint %v doesn't exist in the stream %v", client, in.Id)
				return &pb.StreamResult{}, nil
			}
			endpoint.enabled = in.Enable
			stream.clients[client.String()] = endpoint
			streams[in.Id] = stream
			util.Infof("Client %v updated in stream %v", client, in.Id)
		} else if in.Operation.String() == "DEL_EP" {
			_, ok := stream.clients[client.String()]
			if !ok {
				util.Errorf("Endpoint %v doesn't exist in the stream %v", client, in.Id)
				return &pb.StreamResult{}, nil
			}
			delete(stream.clients, client.String())
			streams[in.Id] = stream
			util.Infof("Client %v deleted from stream %v", client, in.Id)
		}
	}
	return &pb.StreamResult{}, nil
}

func forwardRTPPackets(port uint16) {
	sourceAddr := &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: int(port), Zone: ""}
	sourceConn, err := net.ListenUDP("udp", sourceAddr)
	if err != nil {
		util.Fatalf("Could not start listening on RTP port.", err)
	}
	defer func(sourceConn *net.UDPConn) {
		err := sourceConn.Close()
		if err != nil {
			util.Warningf("Unable to close sourceConn", err)
		}
	}(sourceConn)
	buffer := make([]byte, 65507)
	for {
		n, sourceAddr, err := sourceConn.ReadFromUDP(buffer)
		if err != nil {
			util.Warningf("Error while reading RTP packet.", err)
			continue
		}

		streamID, ok := streamMap[fmt.Sprintf("%s:%d", sourceAddr.IP.String(), sourceAddr.Port)]
		if !ok {
			util.Tracef("RTP stream for server %s:%d not found", sourceAddr.IP.String(), sourceAddr.Port)
			continue
		}
		stream, ok := streams[streamID]
		if !ok {
			util.Errorf("stream %v doesn't exists", streamID)
			continue
		}

		if !sourceAddr.IP.Equal(stream.server.IP) || sourceAddr.Port != stream.server.Port {
			util.Errorf("RTP packet received from unknown server %v, expected %v", sourceAddr, stream.server)
			continue
		}

		for _, endpoint := range stream.clients {
			if endpoint.enabled {
				if _, err := sourceConn.WriteToUDP(buffer[0:n], &endpoint.address); err != nil {
					util.Warningf("Could not forward RTP packet.", err)
				} else {
					util.Tracef("RTP packet sent to %v", endpoint.address)
				}
			}
		}
	}
}

func forwardRTCPPackets(port uint16) {
	sourceAddr := &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: int(port), Zone: ""}
	sourceConn, err := net.ListenUDP("udp", sourceAddr)
	if err != nil {
		util.Fatalf("Could not start listening on RTCP port.", err)
	}
	defer func(sourceConn *net.UDPConn) {
		err := sourceConn.Close()
		if err != nil {
			util.Warningf("Unable to close sourceConn", err)
		}
	}(sourceConn)
	buffer := make([]byte, 65507)
	for {
		n, sourceAddr, err := sourceConn.ReadFromUDP(buffer)
		if err != nil {
			util.Warningf("Error while reading RTCP packet.", err)
			continue
		}

		streamID, ok := streamMap[fmt.Sprintf("%s:%d", sourceAddr.IP.String(), sourceAddr.Port-1)]
		if !ok {
			util.Tracef("RTCP stream for server %s:%d not found", sourceAddr.IP.String(), sourceAddr.Port)
			continue
		}
		stream, ok := streams[streamID]
		if !ok {
			util.Errorf("stream %v doesn't exists", streamID)
			continue
		}

		if !sourceAddr.IP.Equal(stream.server.IP) || sourceAddr.Port != stream.server.Port+1 {
			util.Errorf("RTCP packet received from unknown server %v, expected %v", sourceAddr, stream.server)
			continue
		}

		for _, endpoint := range stream.clients {
			if endpoint.enabled {
				RTCPAddress := net.UDPAddr{IP: endpoint.address.IP, Port: endpoint.address.Port + 1, Zone: endpoint.address.Zone}
				if _, err := sourceConn.WriteToUDP(buffer[0:n], &RTCPAddress); err != nil {
					util.Warningf("Could not forward RTCP packet.", err)
				} else {
					util.Tracef("RTP packet sent to %v", endpoint.address)
				}
			}
		}
	}
}

func main() {
	logLevel := util.GetLogLevelFromEnv()
	logType := util.GetLogTypeFromEnv()

	config := util.LoggerConfig{
		LogLevel: logLevel,
		LogType:  logType,
	}
	util.InitLogger(config)

	flag.Parse()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		util.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterMsmDataPlaneServer(s, &server{})

	healthService := util.NewHealthChecker()
	grpc_health_v1.RegisterHealthServer(s, healthService)

	go forwardRTPPackets(uint16(*rtpPort))
	go forwardRTCPPackets(uint16(*rtpPort + 1))

	util.Infof("Listening for messages coming from CP at %v", lis.Addr())

	if err := s.Serve(lis); err != nil {
		util.Fatalf("failed to serve: %v", err)
	}

	defer func(lis net.Listener) {
		err := lis.Close()
		if err != nil {
			util.Fatalf("failed to close connection with CP: %v", err)
		}
	}(lis)
}
