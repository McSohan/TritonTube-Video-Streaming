package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"tritontube/internal/storage"

	"net"
	pb "tritontube/internal/proto"

	"google.golang.org/grpc"
)

func main() {
	host := flag.String("host", "localhost", "Host address for the server")
	port := flag.Int("port", 8090, "Port number for the server")
	flag.Parse()

	// Validate arguments
	if *port <= 0 {
		panic("Error: Port number must be positive")
	}

	if flag.NArg() < 1 {
		fmt.Println("Usage: storage [OPTIONS] <baseDir>")
		fmt.Println("Error: Base directory argument is required")
		return
	}
	baseDir := flag.Arg(0)

	fmt.Println("Starting storage server...")
	fmt.Printf("Host: %s\n", *host)
	fmt.Printf("Port: %d\n", *port)
	fmt.Printf("Base Directory: %s\n", baseDir)

	// go run ./cmd/storage -host localhost -port 8090 "./storage/8090"

	nodeAddr := fmt.Sprintf("%s:%d", *host, *port)
	storageserver, err := storage.NewStorageService(baseDir)
	if err != nil {
		panic("Unable to create storage")
	}
	grpcServer := grpc.NewServer()
	pb.RegisterStorageServiceServer(grpcServer, storageserver)

	go func() {
		lis, err := net.Listen("tcp", nodeAddr)
		if err != nil {
			log.Fatalf("Failed to listen: %v", err)
		}
		log.Printf("gRPC server listening on %s", nodeAddr)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Wait for ctrl+c to terminate gracefully
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Println("Shutting down gRPC server...")
	grpcServer.GracefulStop()

}
