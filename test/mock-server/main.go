package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dell/csm-replication/test/mock-server/server"
	"github.com/dell/csm-replication/test/mock-server/stub"

	"github.com/dell/dell-csi-extensions/replication"
	"google.golang.org/grpc"
)

var (
	csiAddress string
	stubs      string
	apiPort    string
)

func init() {
	flag.StringVar(&csiAddress, "csi-address", "/var/run/csi.sock", "Address of the grpc server")
	flag.StringVar(&stubs, "stubs", "./stubs", "Location of the stubs directory")
	flag.StringVar(&apiPort, "apiPort", "4771", "API port")
	flag.Parse()
}

func main() {
	// run admin stub server
	stub.RunStubServer(stub.Options{
		StubPath: stubs,
		Port:     apiPort,
		BindAddr: "0.0.0.0",
	})
	var protocol string
	if strings.Contains(csiAddress, ":") {
		protocol = "tcp"
	} else {
		protocol = "unix"
	}
	lis, err := net.Listen(protocol, csiAddress)
	if err != nil {
		log.Fatalf("failed to listen on address [%s]: %s", csiAddress, err.Error())
	}

	s := grpc.NewServer()

	replication.RegisterReplicationServer(s, &server.Replication{})

	fmt.Printf("Serving gRPC on %s\n", csiAddress)
	errChan := make(chan error)
	stopChan := make(chan os.Signal, 1)
	// bind OS events to the signal channel
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// run blocking call in a separate goroutine, report errors via channel
	go func() {
		if err := s.Serve(lis); err != nil {
			errChan <- err
		}
	}()

	// terminate gracefully before leaving main function
	defer func() {
		s.GracefulStop()
		log.Printf("Server stopped gracefully")
	}()

	// block until either OS signal, or server fatal error
	select {
	case err := <-errChan:
		log.Printf("Fatal error: %v\n", err)
	case <-stopChan:
		log.Printf("Server stopped successfully")
	}
}
