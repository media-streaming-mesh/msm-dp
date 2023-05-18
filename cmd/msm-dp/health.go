package main

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	log "github.com/sirupsen/logrus"
)

// TODO - implement some checks to make sure all necessary resources/connections/interfaces are ready
// Split liveness and Readiness checks
const isEverythingReady = true

type HealthChecker struct{}

func NewHealthChecker() *HealthChecker {
	return &HealthChecker{}
}

func (h *HealthChecker) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	log.Info("Serving the Check request for health check")

	if isEverythingReady == true {
		log.Printf("✅ Server's status is %s", grpc_health_v1.HealthCheckResponse_SERVING)
		return &grpc_health_v1.HealthCheckResponse{
			Status: grpc_health_v1.HealthCheckResponse_SERVING,
		}, nil
	} else if isEverythingReady == false {
		log.Printf("🚫 Server's status is %s", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
		return &grpc_health_v1.HealthCheckResponse{
			Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING,
		}, nil
	} else {
		log.Printf("🚫 Server's status is %s", grpc_health_v1.HealthCheckResponse_UNKNOWN)
		return &grpc_health_v1.HealthCheckResponse{
			Status: grpc_health_v1.HealthCheckResponse_UNKNOWN,
		}, nil
	}
}

// Watch is used by clients to receive updates when the service status changes.
// Watch only dummy implemented just to satisfy the interface.
func (h *HealthChecker) Watch(req *grpc_health_v1.HealthCheckRequest, w grpc_health_v1.Health_WatchServer) error {
	return status.Error(codes.Unimplemented, "Watching is not supported")
}
