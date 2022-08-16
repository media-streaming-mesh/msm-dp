package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	"github.com/media-streaming-mesh/msm-dp/cmd/proxy"
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
var serverPort int32
var clientPort int32

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
				clientIP = f.String()
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
	go proxy.Run()
	//go ListenServer()
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

func sendResponse(conn *net.UDPConn, addr *net.UDPAddr) {
	_, err := conn.WriteToUDP([]byte("From server: Hello I got your message "), addr)
	if err != nil {
		log.Printf("Couldn't send response %v", err)
	}
}

func ListenServer() {
	log.Printf("ListenServer()")
	p := make([]byte, 2048)
	addr := net.UDPAddr{
		Port: 8050,
		IP:   net.ParseIP("0.0.0.0"),
	}
	ser, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Printf("Some error ListenUDP %v\n", err)
		return
	}
	for {
		log.Printf("Waiting to Read from UDP")
		_, remoteAddr, err := ser.ReadFromUDP(p)
		log.Printf("Read a message from %v %s \n", remoteAddr, p)
		if err != nil {
			log.Printf("Some error ReadFromUDP %v", err)
			continue
		}
		go sendResponse(ser, remoteAddr)
	}
}

func DialClient() {
	p := make([]byte, 2048)
	conn, err := net.Dial("udp", serverIP+":8000")
	if err != nil {
		log.Printf("Some error %v", err)
		return
	}
	fmt.Fprintf(conn, "Hi UDP Server, How are you doing?")
	_, err = bufio.NewReader(conn).Read(p)
	if err == nil {
		log.Printf("%s\n", p)
	} else {
		log.Printf("Some error %v\n", err)
	}
	conn.Close()
}
