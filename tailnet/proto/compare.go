package proto

import (
	"bytes"

	gProto "google.golang.org/protobuf/proto"
)

// Equal returns true if the nodes have the same contents
func (s *Node) Equal(o *Node) (bool, error) {
	sBytes, err := gProto.Marshal(s)
	if err != nil {
		return false, err
	}
	oBytes, err := gProto.Marshal(o)
	if err != nil {
		return false, err
	}
	return bytes.Equal(sBytes, oBytes), nil
}

func (s *DERPMap) Equal(o *DERPMap) (bool, error) {
	sBytes, err := gProto.Marshal(s)
	if err != nil {
		return false, err
	}
	oBytes, err := gProto.Marshal(o)
	if err != nil {
		return false, err
	}
	return bytes.Equal(sBytes, oBytes), nil
}
