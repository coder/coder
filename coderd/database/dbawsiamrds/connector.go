package dbawsiamrds

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/rds/auth"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"golang.org/x/xerrors"
)

var DriverNotSupportedErr = xerrors.New("driver open method not supported")

var _ driver.Connector = &AwsIamConnector{}
var _ driver.Driver = &AwsIamConnector{}

type AwsIamConnector struct {
	cfg   aws.Config
	dbURL string
}

// NewDB creates a new *sqlx.DB using the aws session from the environment and
// pings postgres to ensure connectivity.
func NewDB(ctx context.Context, dbURL string) (*sqlx.DB, error) {
	c, err := NewConnector(ctx, dbURL)
	if err != nil {
		return nil, xerrors.Errorf("creating connector: %w", err)
	}

	sqlDB := sql.OpenDB(c)
	sqlxDB := sqlx.NewDb(sqlDB, "postgres")

	err = sqlxDB.PingContext(ctx)
	if err != nil {
		return nil, xerrors.Errorf("ping postgres: %w", err)
	}

	return sqlxDB, nil
}

// NewConnector creates a new `AwsIamConnector` using the aws session from the
// environment.
func NewConnector(ctx context.Context, dbURL string) (*AwsIamConnector, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	c := &AwsIamConnector{
		cfg:   cfg,
		dbURL: dbURL,
	}

	return c, nil
}

// Connect fulfills the driver.Connector interface using aws iam rds credentials
// from the environment
func (c *AwsIamConnector) Connect(ctx context.Context) (driver.Conn, error) {
	nURL, err := getAuthenticatedURL(c.cfg, c.dbURL)
	if err != nil {
		return nil, xerrors.Errorf("assigning authentication token to url: %w", err)
	}

	connector, err := pq.NewConnector(nURL)
	if err != nil {
		return nil, xerrors.Errorf("building new pq connector: %w", err)
	}

	conn, err := connector.Connect(ctx)
	if err != nil {
		return nil, xerrors.Errorf("making connection: %w", err)
	}

	return conn, nil
}

// Driver fulfills the driver.Connector interface. It shouldn't be used
func (c *AwsIamConnector) Driver() driver.Driver {
	return c
}

// Open fulfills the driver.Driver interface with an error.
// This interface should not be opened via the driver open method.
func (*AwsIamConnector) Open(_ string) (driver.Conn, error) {
	return nil, DriverNotSupportedErr
}

// getAuthenticatedURL generates an RDS auth token and inserts it into the
// password field of the supplied URL.
func getAuthenticatedURL(cfg aws.Config, dbURL string) (string, error) {
	nURL, err := url.Parse(dbURL)
	if err != nil {
		return "", xerrors.Errorf("parsing dbURL: %w", err)
	}

	// generate a new rds session auth tokenized URL
	rdsEndpoint := fmt.Sprintf("%s:%s", nURL.Hostname(), nURL.Port())
	token, err := auth.BuildAuthToken(context.Background(), rdsEndpoint, cfg.Region, nURL.User.Username(), cfg.Credentials)
	if err != nil {
		return "", xerrors.Errorf("building rds auth token: %w", err)
	}
	// set token as user password
	nURL.User = url.UserPassword(nURL.User.Username(), token)

	return nURL.String(), nil
}
