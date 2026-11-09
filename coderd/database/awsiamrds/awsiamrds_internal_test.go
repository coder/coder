package awsiamrds

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

// failingDriver records the DSN it was asked to open and always fails.
type failingDriver struct {
	gotName string
	err     error
}

func (d *failingDriver) Open(name string) (driver.Conn, error) {
	d.gotName = name
	return nil, d.err
}

// TestOpenErrorOmitsConnectionDetails ensures connection errors do not
// include the connection URL.
func TestOpenErrorOmitsConnectionDetails(t *testing.T) {
	t.Parallel()

	const (
		accessKeyID     = "AKIAEXAMPLEACCESSKEY"
		secretAccessKey = "exampleSecretAccessKey"
	)
	cfg := aws.Config{
		Region:      "us-east-2",
		Credentials: credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	}
	parentErr := xerrors.New("connection refused")
	parent := &failingDriver{err: parentErr}
	d := newDriver(parent, cfg)

	_, err := d.Open("postgres://coder@db.example.com:5432/coder")
	require.Error(t, err)
	require.ErrorIs(t, err, parentErr)

	// Sanity check that the parent received a signed URL, so the assertions
	// below are meaningful.
	require.Contains(t, parent.gotName, accessKeyID)
	require.Contains(t, parent.gotName, "X-Amz-Signature")

	msg := err.Error()
	require.NotContains(t, msg, parent.gotName)
	require.NotContains(t, msg, accessKeyID)
	require.NotContains(t, msg, secretAccessKey)
	require.NotContains(t, msg, "X-Amz-")
}

// TestParseErrorOmitsConnectionDetails ensures URL parse errors do not
// include the connection URL.
func TestParseErrorOmitsConnectionDetails(t *testing.T) {
	t.Parallel()

	const password = "examplePassword"
	cfg := aws.Config{
		Region:      "us-east-2",
		Credentials: credentials.NewStaticCredentialsProvider("AKIAEXAMPLEACCESSKEY", "exampleSecretAccessKey", ""),
	}
	// The non-numeric port makes url.Parse fail.
	dbURL := "postgres://coder:" + password + "@db.example.com:badport/coder"

	d := newDriver(&failingDriver{}, cfg)
	_, err := d.Open(dbURL)
	require.Error(t, err)
	require.NotContains(t, err.Error(), password)

	c, err := d.Connector(dbURL)
	require.NoError(t, err)
	_, err = c.Connect(context.Background())
	require.Error(t, err)
	require.NotContains(t, err.Error(), password)
}
