package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"github.com/lucas-clemente/quic-go"
	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	quick "github.com/media-streaming-mesh/msm-dp/cmd/quic"
	logs "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"log"
	"math/big"
	"net"
	"net/netip"
	"sync"
	"time"
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
	StreamType    uint32
	Encapsulation uint32
	Enable        bool
}

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
				log.Printf("Connection closed, Endpoint Deleted %v", client)
			} else {
				logs.WithError(err).Fatal("unable to find client addr", client)
			}
		}

		log.Printf("Client(s): %+v\n", clients)
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
	//go forwardPackets(uint16(*rtpPort))
	//go forwardPackets(uint16(*rtpPort + 1))
	go quicForwarder()

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

	logs.Printf("socket is %v", sourceConn.LocalAddr().String())

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
		for _, client := range clients {
			if _, err := sourceConn.WriteToUDPAddrPort(buffer[0:n], client.IpAndPort); err != nil {
				logs.WithError(err).Warn("Could not forward packet.")
			}
		}
	}
}

func remove(s []Clients, i int) []Clients {
	if len(s) > 1 {
		s[i] = s[len(s)-1]
		return s[:len(s)-1]
	}

	log.Printf("deleting only entry in the list")
	return nil
}

func SliceIndex(limit int, predicate func(i int) bool) int {
	for i := 0; i < limit; i++ {
		if predicate(i) {
			return i
		}
	}

	log.Printf("unable to find entry in list")
	return -1
}

func generateTLSConfig() (*tls.Config, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	b := pem.Block{Type: "CERTIFICATE", Bytes: certDER}
	certPEM := pem.EncodeToMemory(&b)

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quick"},
	}, nil
}

func quicForwarder() {
	buffer := make([]byte, 65536)

	quicConfig := &quic.Config{
		MaxIdleTimeout: time.Minute,
	}
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic"},
	}

	tlsConf, err := generateTLSConfig()
	if err != nil {
		panic(err)
	}

	ln, err := quick.Listen("udp", ":8050", tlsConf, quicConfig)
	if err != nil {
		panic(err)
	}

	fmt.Println("Waiting for incoming connection")
	conn, err := ln.Accept()
	if err != nil {
		panic(err)
	}
	fmt.Println("Established connection")

	for {
		n, err := conn.Read(buffer)

		if err != nil {
			logs.WithError(err).Error("Could not receive a packet")
			continue
		}
		for _, client := range clients {
			conn, err := quick.Dial(client.IpAndPort.String(), tlsConf, quicConfig)
			if err != nil {
				panic(err)
			}
			if _, err := conn.Write(buffer[0:n]); err != nil {
				logs.WithError(err).Warn("Could not forward packet.")
			}
		}
	}
}
