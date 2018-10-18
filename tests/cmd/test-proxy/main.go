package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"time" // _ "github.com/golang/protobuf/proto"

	"github.com/mwitkow/grpc-proxy/proxy" // codec import
	_ "github.com/mwitkow/grpc-proxy/proxy/codec"
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

	var address, serverAddress string
	var to int
	flag.StringVar(&address, "a", "", "set the listening addess")
	flag.StringVar(&serverAddress, "s", "", "set the target server addess")
	flag.IntVar(&to, "t", 60, "set timeout in seconds")
	flag.Parse()

	if address == "" {
		log.Fatal("address cannot be empty")
	}

	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal("cannot listen on privided address:", err)
	}

	// Setup of the proxy's Director.
	s, err := grpc.Dial(serverAddress, grpc.WithInsecure())
	if err != nil {
		log.Fatal("must not error on deferred client Dial", err)
	}

	director := func(ctx context.Context, fullName string) (context.Context, *grpc.ClientConn, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			if _, exists := md[rejectingMdKey]; exists {
				return ctx, nil, status.Errorf(codes.PermissionDenied, "testing rejection")
			}
		}
		// Explicitly copy the metadata, otherwise the tests will fail.
		outCtx, _ := context.WithCancel(ctx)
		outCtx = metadata.NewOutgoingContext(outCtx, md.Copy())
		return outCtx, s, nil
	}
	p := grpc.NewServer(
		// grpc.CustomCodec(proxy.Codec()),
		grpc.UnknownServiceHandler(proxy.TransparentHandler(director)),
	)
	// Ping handler is handled as an explicit registration and not as a TransparentHandler.
	proxy.RegisterService(p, director,
		"mwitkow.testproto.TestService",
		"Ping")

	go func() {
		// stop after timeout
		<-time.After(time.Second * time.Duration(to))
		os.Exit(0)
	}()

	// encoding.RegisterCodec(NewCodec())

	cd := encoding.GetCodec("grpcproxy")
	if cd == nil {
		log.Fatal("grpcproxy codec not registered")
	}

	log.Printf("starting grpc.Proxy at: %s with codec %s\n", lis.Addr().String(), cd.Name())

	if err := p.Serve(lis); err != nil {
		log.Fatal("proxy failed", err)
	}

}
