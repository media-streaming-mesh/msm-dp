package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/libp2p/go-reuseport"
	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	logs "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"net"
	"strings"
	"sync"
)

var (
	port    = flag.Int("port", 9000, "The server port")
	rtpPort = flag.Int("rtpPort", 8050, "rtp port")
)

var wg sync.WaitGroup

var serverIP string
var clientIPs []string
var localIP string
var serverPort string
var clientPort string

//var clients = make(chan []string)

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
	}
	if in.Operation.String() == "ADD_EP" {
		intValue := 0
		ip, _ := fmt.Sscan(in.Endpoint.Ip, &intValue)
		for i := 0; i <= ip; i++ {
			clientIPs = append(clientIPs, in.Endpoint.Ip)
		}
		log.Printf("Client IP: %v", clientIPs)
		go forwardRtpPackets()
		go forwardRtcpPackets()
	}

	for range clientIPs {
		if in.Operation.String() == "DEL_EP" {
			remove(clientIPs, SliceIndex(len(clientIPs), func(i int) bool { return clientIPs[i] == in.Endpoint.Ip }))
			log.Printf("Connection closed, Endpoint Deleted %v", in.Endpoint.Ip)
		}
	}
	return &pb.StreamResult{
		Success: in.Enable,
	}, nil
}

func main() {
	wg.Add(1)
	getPodsIP()
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
	defer func(lis net.Listener) {
		err := lis.Close()
		if err != nil {
			log.Fatalf("failed to close connection with CP: %v", err)
		}
	}(lis)
}

func forwardRtpPackets() {
	//Listen to data from server pod
	buffer := make([]byte, 65536)

	sourceConn, err := reuseport.Dial("udp", localIP+fmt.Sprintf(":%d", *rtpPort), serverIP+fmt.Sprintf(":%d", *rtpPort))
	if err != nil {
		logs.WithError(err).Fatal("Could not listen on address:", serverIP+fmt.Sprintf(":%d", *rtpPort))
		return
	}

	defer func(sourceConn net.Conn) {
		err := sourceConn.Close()
		if err != nil {
			logs.WithError(err).Fatal("Could not close sourceConn:", err)
		}
	}(sourceConn)

	var targetConn []net.Conn
	for _, v := range clientIPs {
		conn, err := reuseport.Dial("udp", localIP+fmt.Sprintf(":%d", *rtpPort), v+fmt.Sprintf(":%d", *rtpPort))
		if err != nil {
			logs.WithError(err).Fatal("Could not connect to target address:", v)
			return
		}

		defer func(conn net.Conn) {
			err := conn.Close()
			if err != nil {
				logs.WithError(err).Fatal("Could not close conn:", err)
			}
		}(conn)
		targetConn = append(targetConn, conn)
	}

	logs.Printf("===> Starting proxy, Source at %v, Target at %v...", serverIP+fmt.Sprintf(":%d", *rtpPort), clientIPs)

	for {
		n, err := sourceConn.Read(buffer)

		if err != nil {
			logs.WithError(err).Error("Could not receive a packet")
			continue
		}
		for _, v := range targetConn {
			if _, err := v.Write(buffer[0:n]); err != nil {
				logs.WithError(err).Warn("Could not forward packet.")
			}
		}
	}
}
func forwardRtcpPackets() {
	//Listen to data from server pod
	buffer := make([]byte, 65536)

	sourceConn, err := reuseport.Dial("udp", localIP+fmt.Sprintf(":%d", *rtpPort+1), serverIP+fmt.Sprintf(":%d", *rtpPort+1))
	if err != nil {
		logs.WithError(err).Fatal("Could not listen on address:", serverIP+fmt.Sprintf(":%d", *rtpPort+1))
		return
	}

	defer func(sourceConn net.Conn) {
		err := sourceConn.Close()
		if err != nil {
			logs.WithError(err).Fatal("Could close sourceConn:", err)
		}
	}(sourceConn)

	var targetConn []net.Conn
	for _, v := range clientIPs {
		conn, err := reuseport.Dial("udp", localIP+fmt.Sprintf(":%d", *rtpPort+1), v+fmt.Sprintf(":%d", *rtpPort+1))
		if err != nil {
			logs.WithError(err).Fatal("Could not connect to target address:", v)
			return
		}

		defer func(conn net.Conn) {
			err := conn.Close()
			if err != nil {
				logs.WithError(err).Fatal("Could close conn:", err)
			}
		}(conn)
		targetConn = append(targetConn, conn)
	}

	logs.Printf("===> Starting proxy, Source at %v, Target at %v...", serverIP+fmt.Sprintf(":%d", *rtpPort+1), clientIPs)

	for {
		n, err := sourceConn.Read(buffer)

		if err != nil {
			logs.WithError(err).Error("Could not receive a packet")
			continue
		}
		for _, v := range targetConn {
			if _, err := v.Write(buffer[0:n]); err != nil {
				logs.WithError(err).Warn("Could not forward packet.")
			}
		}
	}
}

func getPodsIP() {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientSet
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	pods, err := clientSet.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{})

	if err != nil {
		panic(err.Error())
	}
	//fmt.Printf("There are %d Endpoints in the cluster\n", len(pods.Items))

	for _, pod := range pods.Items {
		// fmt.Printf("%+v\n", ep)
		var podName = strings.Contains(pod.Name, "proxy")
		if podName == true {
			localIP = pod.Status.PodIP
			//fmt.Println(pod.Name, pod.Status.PodIP)
		}
	}
	wg.Done()
}

func remove(s []string, index int) []string {
	return append(s[:index], s[index+1:]...)
}
func SliceIndex(limit int, predicate func(i int) bool) int {
	for i := 0; i < limit; i++ {
		if predicate(i) {
			return i
		}
	}
	return -1
}
