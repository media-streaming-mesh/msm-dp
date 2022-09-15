package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/libp2p/go-reuseport"
	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
)

var (
	port = flag.Int("port", 9000, "The server port")
)

var wg sync.WaitGroup

var serverIP string
var clientIP string
var localIP string

// server is used to implement msm_dp.server.
type server struct {
	pb.UnimplementedMsmDataPlaneServer
}

func (s *server) StreamAddDel(_ context.Context, in *pb.StreamData) (*pb.StreamResult, error) {
	endpoint := reflect.ValueOf(in.Endpoint).Elem()
	protocol := reflect.ValueOf(in.Protocol)

	for i := 0; i < endpoint.NumField(); i++ {
		f := endpoint.Field(i)
		if protocol.Int() == 0 {
			if i == 3 {
				//log.Println("Client IP: ", f)
				clientIP = f.String()
				go ForwardPackets()
			}
			if i == 4 {
				//log.Println("Client Port: ", f)
				//clientPort = f.String()
			}
		}
		if protocol.Int() == 3 {
			if i == 3 {
				//log.Println("Server IP: ", f)
				serverIP = f.String()
			}
			if i == 4 {
				//log.Println("Server Port: ", f)
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
	ser, err := reuseport.Dial("udp", localIP+":8050", serverIP+":8050")
	if err != nil {
		log.Printf("Error connect to client %v", err)
		return
	} else {
		log.Printf("Listening for incoming stream at %v, from server at %v", ser.LocalAddr(), ser.RemoteAddr())
	}

	//Start connection to client pod
	conn, err := reuseport.Dial("udp", localIP+":8050", clientIP+":8050")
	if err != nil {
		log.Printf("Error connect to client %v", err)
		return
	} else {
		log.Printf("Forwarding/Proxing Stream from %v, to client at %v", conn.LocalAddr(), conn.RemoteAddr())
	}
	log.Printf("Waiting to Read Packets")
	for {
		n, err := ser.Read(buffer)
		log.Printf("Reading packets: %v \n", buffer[0:n])
		if err != nil {
			log.Printf("Some error while Reading packets %v", err)
		} else {
			data, err := conn.Write(buffer[0:n])
			if err != nil {
				log.Printf("Couldn't send response %v", err)
			} else {
				log.Printf("Forwarding packets: %v", data)
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
