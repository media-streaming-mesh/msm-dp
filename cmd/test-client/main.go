package main

import (
	"context"
	"flag"
	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"time"
)

const (
	defaultName = "Test Client"
)

var (
	addr   = flag.String("addr", "localhost:9000", "the address to connect to")
	name   = flag.String("name", defaultName, "Name to greet")
	result = flag.Bool("test", true, "test passed")
)

func main() {
	flag.Parse()
	// Set up a connection to the server.
	conn, err := grpc.Dial(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewMsmDataPlaneClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = c.StreamAddDel(ctx, &pb.StreamData{Enable: *result})
	if err != nil {
		log.Fatalf("could not greet: %v", err)
	}
	log.Printf("Greetings: We have succsssfully hit the server")
}
