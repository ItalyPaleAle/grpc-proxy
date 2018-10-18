// Copyright 2017 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package proxy_test

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	_ "github.com/mwitkow/grpc-proxy/proxy/codec"
	pb "github.com/mwitkow/grpc-proxy/testservice" // anonymous codec import
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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

var proxyAddress string

func init() {

	flag.StringVar(&proxyAddress, "p", "", "set the test-proxy address string")
	flag.Parse()
}

// asserting service is implemented on the server side and serves as a handler for stuff
type assertingService struct {
	t *testing.T
}

func (s *assertingService) PingEmpty(ctx context.Context, _ *pb.Empty) (*pb.PingResponse, error) {
	// Check that this call has client's metadata.
	md, ok := metadata.FromIncomingContext(ctx)
	assert.True(s.t, ok, "PingEmpty call must have metadata in context")
	_, ok = md[clientMdKey]
	assert.True(s.t, ok, "PingEmpty call must have clients's custom headers in metadata")
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
			require.NoError(s.t, err, "can't fail reading stream")
			return err
		}
		pong := &pb.PingResponse{Value: ping.Value, Counter: counter}
		if err := stream.Send(pong); err != nil {
			require.NoError(s.t, err, "can't fail sending back a pong")
		}
		counter += 1
	}
	stream.SetTrailer(metadata.Pairs(serverTrailerMdKey, "I like ending turtles."))
	return nil
}

// ProxyHappySuite tests the "happy" path of handling: that everything works in absence of connection issues.
type ProxyHappySuite struct {
	suite.Suite

	client     *grpc.ClientConn
	testClient pb.TestServiceClient
}

func (s *ProxyHappySuite) ctx() context.Context {
	// Make all RPC calls last at most 1 sec, meaning all async issues or deadlock will not kill tests.
	ctx, _ := context.WithTimeout(context.TODO(), 120*time.Second)
	return ctx
}

func (s *ProxyHappySuite) TestPingEmptyCarriesClientMetadata() {
	s.T().Skip()
	ctx := metadata.NewOutgoingContext(s.ctx(), metadata.Pairs(clientMdKey, "true"))
	out, err := s.testClient.PingEmpty(ctx, &pb.Empty{})
	require.NoError(s.T(), err, "PingEmpty should succeed without errors")
	require.Equal(s.T(), &pb.PingResponse{Value: pingDefaultValue, Counter: 42}, out)
}

func (s *ProxyHappySuite) TestPingEmpty_StressTest() {
	for i := 0; i < 50; i++ {
		s.TestPingEmptyCarriesClientMetadata()
	}
}

func (s *ProxyHappySuite) TestPingCarriesServerHeadersAndTrailers() {
	s.T().Skip()
	headerMd := make(metadata.MD)
	trailerMd := make(metadata.MD)
	// This is an awkward calling convention... but meh.
	out, err := s.testClient.Ping(s.ctx(), &pb.PingRequest{Value: "foo"}, grpc.Header(&headerMd), grpc.Trailer(&trailerMd))
	require.NoError(s.T(), err, "Ping should succeed without errors")
	require.Equal(s.T(), &pb.PingResponse{Value: "foo", Counter: 42}, out)
	assert.Contains(s.T(), headerMd, serverHeaderMdKey, "server response headers must contain server data")
	assert.Len(s.T(), trailerMd, 1, "server response trailers must contain server data")
}

func (s *ProxyHappySuite) TestPingErrorPropagatesAppError() {
	s.T().Skip()
	_, err := s.testClient.PingError(s.ctx(), &pb.PingRequest{Value: "foo"})
	require.Error(s.T(), err, "PingError should never succeed")
	st, ok := status.FromError(err)
	assert.Equal(s.T(), ok, true)
	assert.Equal(s.T(), codes.FailedPrecondition, st.Code())
	assert.Equal(s.T(), "Userspace error.", st.Message())
}

func (s *ProxyHappySuite) TestDirectorErrorIsPropagated() {
	// See SetupSuite where the StreamDirector has a special case.
	ctx := metadata.NewOutgoingContext(s.ctx(), metadata.Pairs(rejectingMdKey, "true"))
	_, err := s.testClient.Ping(ctx, &pb.PingRequest{Value: "foo"})
	require.Error(s.T(), err, "Director should reject this RPC")
	st, ok := status.FromError(err)
	assert.Equal(s.T(), ok, true)
	assert.Equal(s.T(), codes.PermissionDenied, st.Code())
	assert.Equal(s.T(), "testing rejection", st.Message())
}

func (s *ProxyHappySuite) TestPingStream_FullDuplexWorks() {
	// s.T().Skip()
	stream, err := s.testClient.PingStream(s.ctx())
	require.NoError(s.T(), err, "PingStream request should be successful.")

	for i := 0; i < countListResponses; i++ {
		ping := &pb.PingRequest{Value: fmt.Sprintf("foo:%d", i)}
		require.NoError(s.T(), stream.Send(ping), "sending to PingStream must not fail")
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if i == 0 {
			// Check that the header arrives before all entries.
			headerMd, err := stream.Header()
			require.NoError(s.T(), err, "PingStream headers should not error.")
			assert.Contains(s.T(), headerMd, serverHeaderMdKey, "PingStream response headers user contain metadata")
		}
		require.NotNil(s.T(), resp, "response must not be nil")
		assert.EqualValues(s.T(), i, resp.Counter, "ping roundtrip must succeed with the correct id")
	}
	require.NoError(s.T(), stream.CloseSend(), "no error on close send")
	_, err = stream.Recv()
	require.Equal(s.T(), io.EOF, err, "stream should close with io.EOF, meaining OK")
	// Check that the trailer headers are here.
	trailerMd := stream.Trailer()
	assert.Len(s.T(), trailerMd, 1, "PingList trailer headers user contain metadata")
}

func (s *ProxyHappySuite) TestPingStream_StressTest() {
	for i := 0; i < 50; i++ {
		s.TestPingStream_FullDuplexWorks()
	}
}

func (s *ProxyHappySuite) SetupSuite() {
	var err error

	require.NotEmpty(s.T(), proxyAddress)

	clientConn, err := grpc.Dial(strings.Replace(proxyAddress, "127.0.0.1", "localhost", 1), grpc.WithInsecure(), grpc.WithTimeout(1*time.Second), grpc.WithDefaultCallOptions(grpc.CallContentSubtype("grpcproxy")))
	require.NoError(s.T(), err, "must not error on deferred client Dial")
	s.testClient = pb.NewTestServiceClient(clientConn)
}

func (s *ProxyHappySuite) TearDownSuite() {
	if s.client != nil {
		s.client.Close()
	}
}

func TestProxyHappySuite(t *testing.T) {
	suite.Run(t, &ProxyHappySuite{})
}
