package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/libp2p/go-reuseport"
	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	"google.golang.org/grpc"
	"log"
	"net"
	"reflect"
)

var (
	port = flag.Int("port", 9000, "The server port")
)

var serverIP string
var clientIP string
var serverPort string
var clientPort string

// server is used to implement msm_dp.server.
type server struct {
	pb.UnimplementedMsmDataPlaneServer
}

func (s *server) StreamAddDel(ctx context.Context, in *pb.StreamData) (*pb.StreamResult, error) {
	endpoint := reflect.ValueOf(in.Endpoint).Elem()
	protocol := reflect.ValueOf(in.Protocol)
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
				clientIP = f.String()
				go ForwardPackets()
			}
			if i == 4 {
				log.Println("Client Port: ", f)
				//clientPort = f.String()
			}
		}
		if protocol.Int() == 3 {
			if i == 3 {
				log.Println("Server IP: ", f)
				serverIP = f.String()
			}
			if i == 4 {
				log.Println("Server Port: ", f)
				//serverPort = f.String()

			}
		}

		//TODO: handle other protocols, 1: UDP and 2: QUIC
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
	log.Printf("Started connection with CP at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func ForwardPackets() {
	//Listen to data from server pod
	buffer := make([]byte, 2048)
	ser, err := reuseport.Dial("udp", "172.18.0.2:8050", serverIP+":8050")
	if err != nil {
		log.Printf("Error connect to client %v", err)
		return
	} else {
		log.Printf("Listening for incoming stream at %v, from server at %v", ser.LocalAddr(), ser.RemoteAddr())
	}

	//Start connection to client pod
	conn, err := reuseport.Dial("udp", "172.18.0.2:8050", clientIP+":8050")
	if err != nil {
		log.Printf("Error connect to client %v", err)
		return
	} else {
		log.Printf("Forwarding/Proxing Stream from %v, to client at %v", conn.LocalAddr(), conn.RemoteAddr())
	}
	log.Printf("Waiting to Read Packets")
	for {
		_, err := ser.Read(buffer)
		log.Printf("Reading packets: %v \n", len(buffer))
		if err != nil {
			log.Printf("Some error while Reading packets %v", err)
		} else {
			data, err := conn.Write(buffer)
			if err != nil {
				log.Printf("Couldn't send response %v", err)
			} else {
				log.Printf("Forwarding packets: %v", data)
			}
		}
	}
}
