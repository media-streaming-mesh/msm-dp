package main

import (
	"context"
	"flag"
	"fmt"
	"net"

	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
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
	server  *net.UDPAddr
	clients []Endpoint
}

var streams = make(map[uint32]Stream)

var flows = make(map[*net.UDPAddr][]net.UDPAddr)

// server is used to implement msm_dp.server
type server struct {
	pb.UnimplementedMsmDataPlaneServer
}

func AddressEqual(first *net.UDPAddr, second *net.UDPAddr) bool {
	return first.IP.Equal(second.IP) && first.Port == second.Port && first.Zone == second.Zone
}

func (s *server) StreamAddDel(_ context.Context, in *pb.StreamData) (*pb.StreamResult, error) {
	log.Debugf("Received: message from CP --> Operation = %v", in.Operation)
	switch in.Operation.String() {
	case "CREATE":
		// check if the stream already exists in the streams array
		_, exists := streams[in.Id]

		if !exists {
			streams[in.Id] = Stream{server: &net.UDPAddr{IP: net.ParseIP(in.Endpoint.Ip), Port: int(in.Endpoint.Port), Zone: ""}, clients: []Endpoint{}}
			log.Infof("New stream ID: %v, source %v:%v", in.Id, in.Endpoint.Ip, in.Endpoint.Port)
			//flows[streams[in.Id].server] = []net.UDPAddr{*streams[in.Id].server}

			flow, ok := flows[streams[in.Id].server]
			if ok {
				flow = append(flow, *streams[in.Id].server)
			} else {
				log.Infof("no data-plane flow found for stream %v", in.Id)
			}
			log.Debugf("flows: %v", flows)
			log.Debugf("streams: %v", streams)
		} else {
			log.Errorf("Stream with ID %d already exists", in.Id)
		}
	case "UPDATE":
		log.Errorf("unexpected UPDATE")
	case "DELETE":
		delete(streams, in.Id)
		delete(flows, streams[in.Id].server)
		log.Infof("Deleted stream ID: %v", in.Id)
	default:
		client := &net.UDPAddr{IP: net.ParseIP(in.Endpoint.Ip), Port: int(in.Endpoint.Port), Zone: ""}
		stream := streams[in.Id]

		if in.Operation.String() == "ADD_EP" {
			clients := append(streams[in.Id].clients, Endpoint{enabled: in.Enable, address: *client})
			stream.clients = clients
			flows[streams[in.Id].server] = append(flows[streams[in.Id].server], *client)
			log.Infof("Client %v added to stream %v", client, in.Id)
			log.Debugf("flows: %v", flows)
		} else if in.Operation.String() == "UPD_EP" {
			for _, endpoint := range streams[in.Id].clients {
				if AddressEqual(&endpoint.address, client) {
					endpoint.enabled = in.Enable
					if in.Enable {
						flow, ok := flows[streams[in.Id].server]
						if ok {
							flow = append(flow, *client)
						} else {
							log.Infof("no data-plane flow found for stream %v", in.Id)
						}
					} else {
						for i, flow_client := range flows[streams[in.Id].server] {
							if AddressEqual(&flow_client, client) {
								flows[streams[in.Id].server] = append(flows[streams[in.Id].server][:i], flows[streams[in.Id].server][i+1:]...)
								break
							}
						}
					}
					break
				}
			}
		} else if in.Operation.String() == "DEL_EP" {
			for i, endpoint := range streams[in.Id].clients {
				if AddressEqual(&endpoint.address, client) {
					stream.clients = append(stream.clients[:i], stream.clients[i+1:]...)
					break
				}
			}
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
		log.Debugf("Forwarding packet from %v:%d", sourceAddr.IP.String(), sourceAddr.Port)
		log.Debugf("flows: %v", flows)
		clients, ok := flows[sourceAddr]
		if !ok {
			for _, client := range clients {
				if _, err := sourceConn.WriteToUDP(buffer[0:n], &client); err != nil {
					log.WithError(err).Warn("Could not forward packet.")
				} else {
					log.Trace("sent to ", client)
				}
			}
		} else {
			log.Debugf("unable to find source address %v:%d", sourceAddr.IP.String(), sourceAddr.Port)
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
