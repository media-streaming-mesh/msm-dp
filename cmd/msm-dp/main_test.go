package main

import (
	"context"
	pb "github.com/media-streaming-mesh/msm-dp/api/v1alpha1/msm_dp"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"net"
	"testing"
	"time"
)

type testServer struct {
	pb.UnimplementedMsmDataPlaneServer
}

func (s *testServer) StreamAddDel(_ context.Context, in *pb.StreamData) (*pb.StreamResult, error) {
	log.Printf("Received: message from client Endpoint = %v", in.Endpoint)
	log.Printf("Received: message from client Enable = %v", in.Enable)
	log.Printf("Received: message from client Protocol = %v", in.Protocol)
	log.Printf("Received: message from client Id = %v", in.Id)
	log.Printf("Received: message from client Operation = %v", in.Operation)
	return &pb.StreamResult{
		Success: true,
	}, nil
}

func TestConnectionCP(t *testing.T) {
	lis, err := net.Listen("tcp", ":")
	require.NoError(t, err)
	s := grpc.NewServer()
	pb.RegisterMsmDataPlaneServer(s, &testServer{})
	log.Printf("Started test connection at %v", lis.Addr())
	go func() {
		err := s.Serve(lis)
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
	}()
	defer s.Stop()

	log.Println("Dialing (test CP) ...")
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func(conn *grpc.ClientConn) {
		err := conn.Close()
		if err != nil {
			log.Fatalf("failed to dial: %v", err)
		}
	}(conn)
	c := pb.NewMsmDataPlaneClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	log.Println("Making gRPC request...")
	r, err := c.StreamAddDel(ctx, &pb.StreamData{})
	require.NoError(t, err)
	log.Printf("Testing: %s", r)
}
