package echo

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/xerrors"

	protobuf "google.golang.org/protobuf/proto"

	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

var (
	// ParseComplete is a helper to indicate an empty parse completion.
	ParseComplete = []*proto.Parse_Response{{
		Type: &proto.Parse_Response_Complete{
			Complete: &proto.Parse_Complete{},
		},
	}}
	// ProvisionComplete is a helper to indicate an empty provision completion.
	ProvisionComplete = []*proto.Provision_Response{{
		Type: &proto.Provision_Response_Complete{
			Complete: &proto.Provision_Complete{},
		},
	}}
)

// Serve starts the echo provisioner.
func Serve(ctx context.Context, options *provisionersdk.ServeOptions) error {
	return provisionersdk.Serve(ctx, &echo{}, options)
}

// The echo provisioner serves as a dummy provisioner primarily
// used for testing. It echos responses from JSON files in the
// format %d.protobuf. It's used for testing.
type echo struct {
}

// Parse reads requests from the provided directory to stream responses.
func (*echo) Parse(request *proto.Parse_Request, stream proto.DRPCProvisioner_ParseStream) error {
	for index := 0; ; index++ {
		path := filepath.Join(request.Directory, fmt.Sprintf("%d.parse.protobuf", index))
		_, err := os.Stat(path)
		if err != nil {
			break
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return xerrors.Errorf("read file %q: %w", path, err)
		}
		var response proto.Parse_Response
		err = protobuf.Unmarshal(data, &response)
		if err != nil {
			return xerrors.Errorf("unmarshal: %w", err)
		}
		err = stream.Send(&response)
		if err != nil {
			return err
		}
	}
	return nil
}

// Provision reads requests from the provided directory to stream responses.
func (*echo) Provision(request *proto.Provision_Request, stream proto.DRPCProvisioner_ProvisionStream) error {
	for index := 0; ; index++ {
		path := filepath.Join(request.Directory, fmt.Sprintf("%d.provision.protobuf", index))
		_, err := os.Stat(path)
		if err != nil {
			break
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return xerrors.Errorf("read file %q: %w", path, err)
		}
		var response proto.Provision_Response
		err = protobuf.Unmarshal(data, &response)
		if err != nil {
			return xerrors.Errorf("unmarshal: %w", err)
		}
		err = stream.Send(&response)
		if err != nil {
			return err
		}
	}
	return nil
}

// Tar returns a tar archive of responses to provisioner operations.
func Tar(parseResponses []*proto.Parse_Response, provisionResponses []*proto.Provision_Response) ([]byte, error) {
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for index, response := range parseResponses {
		data, err := protobuf.Marshal(response)
		if err != nil {
			return nil, err
		}
		err = writer.WriteHeader(&tar.Header{
			Name: fmt.Sprintf("%d.parse.protobuf", index),
			Size: int64(len(data)),
		})
		if err != nil {
			return nil, err
		}
		_, err = writer.Write(data)
		if err != nil {
			return nil, err
		}
	}
	for index, response := range provisionResponses {
		data, err := protobuf.Marshal(response)
		if err != nil {
			return nil, err
		}
		err = writer.WriteHeader(&tar.Header{
			Name: fmt.Sprintf("%d.provision.protobuf", index),
			Size: int64(len(data)),
		})
		if err != nil {
			return nil, err
		}
		_, err = writer.Write(data)
		if err != nil {
			return nil, err
		}
	}
	err := writer.Flush()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
