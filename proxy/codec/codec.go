package proxy

import (
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/encoding"
)

func init() {
	encoding.RegisterCodec(NewCodec())
}

const (
	codecName      = "grpcproxy"
	protoCodecName = "proto"
)

// NewCodec returns a proxying encoding.Codec with the default protobuf codec as parent.
//
// See CodecWithParent.
func NewCodec() encoding.Codec {
	return CodecWithParent(protoCodec{})
}

// CodecWithParent returns a proxying encoding.Codec with a user provided codec as parent.
//
// This codec is *crucial* to the functioning of the proxy. It allows the proxy server to be oblivious
// to the schema of the forwarded messages. It basically treats a gRPC message frame as raw bytes.
// However, if the server handler, or the client caller are not proxy-internal functions it will fall back
// to trying to decode the message using a fallback codec.
func CodecWithParent(fallback encoding.Codec) encoding.Codec {
	return &rawCodec{fallback}
}

type rawCodec struct {
	parentCodec encoding.Codec
}

type Frame struct {
	payload []byte
}

func (c *rawCodec) Marshal(v interface{}) ([]byte, error) {
	out, ok := v.(*Frame)
	if !ok {
		return c.parentCodec.Marshal(v)
		// if pm, ok := v.(proto.Marshaler); ok {
		// 	// object can marshal itself, no need for buffer
		// 	return pm.Marshal()
		// }
		// return nil, fmt.Errorf("%s is not a proto.Marshaler", reflect.TypeOf(v).String())
	}
	return out.payload, nil

}

func (c *rawCodec) Unmarshal(data []byte, v interface{}) error {

	dst, ok := v.(*Frame)
	if !ok {
		return c.parentCodec.Unmarshal(data, v)
		// protoMsg := v.(proto.Message)
		// protoMsg.Reset()

		// if pu, ok := protoMsg.(proto.Unmarshaler); ok {
		// 	// object can unmarshal itself, no need for buffer
		// 	return pu.Unmarshal(data)
		// }
		// return fmt.Errorf("%s is not a proto.Marshaler", reflect.TypeOf(v).String())
	}
	dst.payload = data
	return nil

}

func (c *rawCodec) Name() string {
	// return fmt.Sprintf("%s>%s", codecName, c.parentCodec.Name())
	return codecName
}

// protoCodec is a Codec implementation with protobuf. It is the default rawCodec for gRPC.
type protoCodec struct{}

func (protoCodec) Marshal(v interface{}) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

func (protoCodec) Unmarshal(data []byte, v interface{}) error {
	return proto.Unmarshal(data, v.(proto.Message))
}

func (protoCodec) Name() string {
	return protoCodecName
}
