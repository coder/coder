package objstore

import (
	"context"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gocloud.dev/blob"
	"gocloud.dev/blob/fileblob"
	"gocloud.dev/blob/gcsblob"
	"gocloud.dev/blob/s3blob"
	"gocloud.dev/gcp"
	"golang.org/x/oauth2/google"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// Backend enumerates the supported storage backends.
type Backend string

const (
	BackendLocal Backend = "local"
	BackendS3    Backend = "s3"
	BackendGCS   Backend = "gcs"
)

// LocalConfig configures the local filesystem backend.
type LocalConfig struct {
	// Dir is the root directory for stored objects. The directory
	// is created if it does not exist.
	Dir string
}

// S3Config configures an S3-compatible backend.
type S3Config struct {
	Bucket string
	Region string
	// Prefix is an optional key prefix within the bucket.
	Prefix string
	// Endpoint is a custom S3-compatible endpoint (e.g. MinIO, R2).
	// Leave empty for standard AWS S3.
	Endpoint string
}

// GCSConfig configures a Google Cloud Storage backend.
type GCSConfig struct {
	Bucket string
	// Prefix is an optional key prefix within the bucket.
	Prefix string
	// CredentialsFile is an optional path to a service account key
	// file. If empty, Application Default Credentials are used.
	CredentialsFile string
}

// NewLocal creates a Store backed by the local filesystem.
func NewLocal(cfg LocalConfig) (Store, error) {
	if cfg.Dir == "" {
		return nil, xerrors.New("local object store directory is required")
	}

	if err := os.MkdirAll(cfg.Dir, 0o700); err != nil {
		return nil, xerrors.Errorf("create object store directory %q: %w", cfg.Dir, err)
	}

	bucket, err := fileblob.OpenBucket(cfg.Dir, &fileblob.Options{
		// Place temp files next to the target files instead of
		// os.TempDir. This avoids EXDEV (cross-device link) errors
		// when the storage directory is on a different filesystem.
		NoTempDir: true,
		// We handle metadata in the database, not in sidecar files.
		Metadata: fileblob.MetadataDontWrite,
	})
	if err != nil {
		return nil, xerrors.Errorf("open local bucket at %q: %w", cfg.Dir, err)
	}

	return newPrefixed(bucket, ""), nil
}

// NewS3 creates a Store backed by an S3-compatible service.
func NewS3(ctx context.Context, cfg S3Config) (Store, error) {
	if cfg.Bucket == "" {
		return nil, xerrors.New("S3 bucket name is required")
	}

	opts := []func(*awsconfig.LoadOptions) error{}
	if cfg.Region != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.Region))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, xerrors.Errorf("load AWS config: %w", err)
	}

	s3Opts := []func(*s3.Options){}
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)

	bucket, err := s3blob.OpenBucket(ctx, client, cfg.Bucket, nil)
	if err != nil {
		return nil, xerrors.Errorf("open S3 bucket %q: %w", cfg.Bucket, err)
	}

	return newPrefixed(bucket, cfg.Prefix), nil
}

// NewGCS creates a Store backed by Google Cloud Storage.
func NewGCS(ctx context.Context, cfg GCSConfig) (Store, error) {
	if cfg.Bucket == "" {
		return nil, xerrors.New("GCS bucket name is required")
	}

	var creds *google.Credentials
	var err error

	if cfg.CredentialsFile != "" {
		jsonData, err := os.ReadFile(cfg.CredentialsFile)
		if err != nil {
			return nil, xerrors.Errorf("read GCS credentials file %q: %w", cfg.CredentialsFile, err)
		}
		//nolint:staticcheck // CredentialsFromJSON is the standard way to load service account keys.
		creds, err = google.CredentialsFromJSON(ctx, jsonData, "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			return nil, xerrors.Errorf("parse GCS credentials: %w", err)
		}
	} else {
		creds, err = gcp.DefaultCredentials(ctx)
		if err != nil {
			return nil, xerrors.Errorf("obtain GCP default credentials: %w", err)
		}
	}

	gcpClient, err := gcp.NewHTTPClient(gcp.DefaultTransport(), gcp.CredentialsTokenSource(creds))
	if err != nil {
		return nil, xerrors.Errorf("create GCP HTTP client: %w", err)
	}

	bucket, err := gcsblob.OpenBucket(ctx, gcpClient, cfg.Bucket, nil)
	if err != nil {
		return nil, xerrors.Errorf("open GCS bucket %q: %w", cfg.Bucket, err)
	}

	return newPrefixed(bucket, cfg.Prefix), nil
}

// newPrefixed wraps a bucket with an optional key prefix and returns
// a Store.
func newPrefixed(bucket *blob.Bucket, prefix string) Store {
	if prefix != "" {
		bucket = blob.PrefixedBucket(bucket, prefix+"/")
	}
	return New(bucket)
}

// FromConfig creates a Store from deployment configuration. The
// configDir is the Coder config directory (e.g. ~/.config/coderv2)
// and is used as the default root when the local backend is selected
// without an explicit directory.
func FromConfig(ctx context.Context, cfg codersdk.ObjectStoreConfig, configDir string) (Store, error) {
	switch Backend(cfg.Backend.String()) {
	case BackendLocal, "":
		dir := cfg.LocalDir.String()
		if dir == "" {
			dir = filepath.Join(configDir, "objectstore")
		}
		return NewLocal(LocalConfig{Dir: dir})

	case BackendS3:
		return NewS3(ctx, S3Config{
			Bucket:   cfg.S3Bucket.String(),
			Region:   cfg.S3Region.String(),
			Prefix:   cfg.S3Prefix.String(),
			Endpoint: cfg.S3Endpoint.String(),
		})

	case BackendGCS:
		return NewGCS(ctx, GCSConfig{
			Bucket:          cfg.GCSBucket.String(),
			Prefix:          cfg.GCSPrefix.String(),
			CredentialsFile: cfg.GCSCredentialsFile.String(),
		})

	default:
		return nil, xerrors.Errorf("unknown object store backend: %q", cfg.Backend.String())
	}
}
