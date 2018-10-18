package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	_ "github.com/mwitkow/grpc-proxy/proxy/codec"
	pb "github.com/mwitkow/grpc-proxy/testservice"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	pingDefaultValue   = "I like kittens."
	clientMdKey        = "test-client-header"
	serverHeaderMdKey  = "test-client-header"
	serverTrailerMdKey = "test-client-trailer"

	rejectingMdKey = "test-reject-rpc-if-in-context"

	countListResponses = 20
)

func main() {

	var address string
	var to int
	flag.StringVar(&address, "a", "", "set the listening addess")
	flag.IntVar(&to, "t", 60, "set timeout in seconds")
	flag.Parse()

	if address == "" {
		log.Fatal("address cannot be empty")
	}

	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal("cannot listen on provided address:", err)
	}

	go func() {
		// stop after timeout
		<-time.After(time.Second * time.Duration(to))
		os.Exit(0)
	}()

	cd := encoding.GetCodec("grpcproxy")
	if cd == nil {
		log.Fatal("grpcproxy codec not registered")
	}

	s := grpc.NewServer()
	pb.RegisterTestServiceServer(s, &assertingService{})
	if err := s.Serve(lis); err != nil {
		log.Fatal(err)
	}

}

// asserting service is implemented on the server side and serves as a handler for stuff
type assertingService struct {
}

func (s *assertingService) PingEmpty(ctx context.Context, _ *pb.Empty) (*pb.PingResponse, error) {
	// Check that this call has client's metadata.
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, grpc.Errorf(codes.Unknown, "PingEmpty call must have metadata in context")
	}
	_, ok = md[clientMdKey]
	if !ok {
		return nil, grpc.Errorf(codes.Unknown, "PingEmpty call must have clients's custom headers in metadata")
	}
	return &pb.PingResponse{Value: pingDefaultValue, Counter: 42}, nil
}

func (s *assertingService) Ping(ctx context.Context, ping *pb.PingRequest) (*pb.PingResponse, error) {
	// Send user trailers and headers.
	grpc.SendHeader(ctx, metadata.Pairs(serverHeaderMdKey, "I like turtles."))
	grpc.SetTrailer(ctx, metadata.Pairs(serverTrailerMdKey, "I like ending turtles."))
	return &pb.PingResponse{Value: ping.Value, Counter: 42}, nil
}

func (s *assertingService) PingError(ctx context.Context, ping *pb.PingRequest) (*pb.Empty, error) {
	return nil, status.Errorf(codes.FailedPrecondition, "Userspace error.")
}

func (s *assertingService) PingList(ping *pb.PingRequest, stream pb.TestService_PingListServer) error {
	// Send user trailers and headers.
	stream.SendHeader(metadata.Pairs(serverHeaderMdKey, "I like turtles."))
	for i := 0; i < countListResponses; i++ {
		stream.Send(&pb.PingResponse{Value: ping.Value, Counter: int32(i)})
	}
	stream.SetTrailer(metadata.Pairs(serverTrailerMdKey, "I like ending turtles."))
	return nil
}

func (s *assertingService) PingStream(stream pb.TestService_PingStreamServer) error {
	stream.SendHeader(metadata.Pairs(serverHeaderMdKey, "I like turtles."))
	counter := int32(0)
	for {
		ping, err := stream.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			// require.NoError(s.t, err, "can't fail reading stream")
			return fmt.Errorf("can't fail reading stream: %s", err.Error())
		}
		pong := &pb.PingResponse{Value: ping.Value, Counter: counter}
		if err := stream.Send(pong); err != nil {
			return fmt.Errorf("can't fail sending back a pong: %s", err.Error())
		}
		counter++
	}
	stream.SetTrailer(metadata.Pairs(serverTrailerMdKey, "I like ending turtles."))
	return nil
}
