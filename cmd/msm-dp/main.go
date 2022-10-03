package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/libp2p/go-reuseport"
	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	"google.golang.org/grpc"
	"io"
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
var clients = make(chan []string)

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
		clientIPs = append(clientIPs, in.Endpoint.Ip)
		go func() {
			clients <- clientIPs
		}()
		log.Printf("Client IP: %v", clientIPs)
		for range clientIPs {
			go ForwardPackets()
		}

	}
	if in.Operation.String() == "DEL_EP" {
		remove(clientIPs, in.Endpoint.Ip)
		log.Printf("Connection closed, Endpoint Deleted %v", in.Endpoint.Ip)
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
}

func ForwardPackets() {
	//Listen to data from server pod
	buffer := make([]byte, 65536)
	go rtcpConnection()
	ser, err := reuseport.Dial("udp", localIP+fmt.Sprintf(":%d", *rtpPort), serverIP+fmt.Sprintf(":%d", *rtpPort))
	if err != nil {
		log.Printf("Error connect to server %v", err)
		return
	} else {
		log.Printf("Listening for RTP incoming stream at %v, from server at %v", ser.LocalAddr(), ser.RemoteAddr())
	}

	for _, clientIP := range <-clients {
		fmt.Println(clientIP)
		//Start connection to client pod
		conn, err := reuseport.Dial("udp", localIP+fmt.Sprintf(":%d", *rtpPort), clientIP+fmt.Sprintf(":%d", *rtpPort))
		if err != nil {
			log.Printf("Error connect to client %v", err)
			return
		} else {
			log.Printf("Forwarding/Proxing RTP Stream from %v, to client at %v", conn.LocalAddr(), conn.RemoteAddr())
		}
		log.Printf("Waiting to Read Packets")
		log.Printf("Proxying check Stub logs")

		for {
			n, err := ser.Read(buffer)
			//log.Printf("Reading packets: %v \n", buffer[0:n])
			if err != nil {
				log.Printf("Some error while Reading packets %v", err)
			} else {
				_, err := conn.Write(buffer[0:n])
				if err != nil {
					log.Printf("Couldn't send response %v", err)
				}
				//} else {
				//	log.Printf("Forwarding packets: %v", data)
				//}
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

func remove[T comparable](l []T, item T) []T {
	for i, other := range l {
		if other == item {
			return append(l[:i], l[i+1:]...)
		}
	}
	return l
}

func handleStream(connSrc net.Conn, connDest net.Conn) {
	defer connSrc.Close()
	for {
		//var bufferSrc []byte
		bufferSrc := make([]byte, 65536)
		srcRSize, errSrcR := connSrc.Read(bufferSrc)
		if errSrcR == io.EOF || errSrcR != nil {
			log.Println("Read error:", errSrcR)
			break
		}
		srcWSize, errSrcW := connDest.Write(bufferSrc[0:srcRSize])
		if errSrcW != nil {
			log.Println("Write error:", errSrcW)
			return
		}
		log.Println("size", srcRSize, srcWSize)
	}
}

func handleConnection(connSrc net.Conn, rAddr string) {
	connDest, err := reuseport.Dial("udp", localIP+fmt.Sprintf(":%d", *rtpPort+1), rAddr+fmt.Sprintf(":%d", *rtpPort+1))
	log.Printf("Forwarding/Proxing RTCP Stream from %v, to client at %v", connDest.LocalAddr(), connDest.RemoteAddr())

	if err != nil {
		log.Println("Dial error:", err)
		return
	}
	go handleStream(connSrc, connDest)
	go handleStream(connDest, connSrc)
}

func rtcpConnection() {

	ln, err := reuseport.Dial("udp", localIP+fmt.Sprintf(":%d", *rtpPort+1), serverIP+fmt.Sprintf(":%d", *rtpPort+1))
	log.Printf("Listening for RTCP incoming stream at %v, from server at %v", ln.LocalAddr(), ln.RemoteAddr())

	if err != nil {
		log.Println("Listen error:", err)
		return
	}
	for {
		for _, clientIP := range <-clients {
			go handleConnection(ln, clientIP)
		}
	}
}
