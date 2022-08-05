package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"reflect"

	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	"google.golang.org/grpc"
)

var (
	port = flag.Int("port", 9000, "The server port")
)

// server is used to implement msm_dp.server.
type server struct {
	pb.UnimplementedMsmDataPlaneServer
}

func (s *server) StreamAddDel(ctx context.Context, in *pb.StreamData) (*pb.StreamResult, error) {
	log.Printf("Received: message from client Endpoint = %v", in.Endpoint)
	log.Printf("Received: message from client Enable = %v", in.Enable)
	log.Printf("Received: message from client Protocol = %v", in.Protocol)
	log.Printf("Received: message from client Id = %v", in.Id)
	log.Printf("Received: message from client Operation = %v", in.Operation)
	//log.Printf("Received: message from client Context = %v", ctx)

	endpoint := reflect.ValueOf(in.Endpoint).Elem()
	//////endpoint := reflect.ValueOf(in.Endpoint)
	//log.Println("Endpoint: ", endpoint)
	protocol := reflect.ValueOf(in.Protocol)
	//log.Println("Protocol: ", protocol)
	switch protocol.Int() {
	case 0:
		log.Println("Protocol 0: ", protocol)
	case 1:
		log.Println("Protocol 1: ", protocol)
	case 2:
		log.Println("Protocol 2: ", protocol)
	case 3:
		log.Println("Protocol 3: ", protocol)
	default:
		log.Println("Protocol Error")

	}
	for i := 0; i < endpoint.NumField(); i++ {
		f := endpoint.Field(i)
		if protocol.Int() == 0 {
			if i == 3 {
				log.Println("Client IP: ", f)
			}
			if i == 4 {
				log.Println("Client Port: ", f)
			}

		}
		if protocol.Int() == 3 {
			if i == 3 {
				log.Println("Server IP: ", f)
			}
			if i == 4 {
				log.Println("Server Port: ", f)
			}
		}

		//TODO: handle protocol 1: UDP and 2: QUIC
		if protocol.Int() == 1 {

		}
		if protocol.Int() == 2 {

		}
	}

	return &pb.StreamResult{
		Success: true,
	}, nil
}

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterMsmDataPlaneServer(s, &server{})
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
