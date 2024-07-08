package awsiamrds

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/rds/auth"
	"golang.org/x/xerrors"
)

type awsIamRdsDriver struct {
	parent driver.Driver
	cfg    aws.Config
}

var _ driver.Driver = &awsIamRdsDriver{}

// Register initializes and registers our aws iam rds wrapped database driver.
func Register(ctx context.Context, parentName string) (string, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", err
	}

	db, err := sql.Open(parentName, "")
	if err != nil {
		return "", err
	}

	// create a new aws iam rds driver
	d := newDriver(db.Driver(), cfg)
	name := fmt.Sprintf("%s-awsiamrds", parentName)
	sql.Register(fmt.Sprintf("%s-awsiamrds", parentName), d)

	return name, nil
}

// newDriver will create a new *AwsIamRdsDriver using the environment aws session.
func newDriver(parentDriver driver.Driver, cfg aws.Config) *awsIamRdsDriver {
	return &awsIamRdsDriver{
		parent: parentDriver,
		cfg:    cfg,
	}
}

// Open creates a new connection to the database using the provided name.
func (d *awsIamRdsDriver) Open(name string) (driver.Conn, error) {
	// set password with signed aws authentication token for the rds instance
	nURL, err := getAuthenticatedURL(d.cfg, name)
	if err != nil {
		return nil, xerrors.Errorf("assigning authentication token to url: %w", err)
	}

	// make connection
	conn, err := d.parent.Open(nURL)
	if err != nil {
		return nil, xerrors.Errorf("opening connection with %s: %w", nURL, err)
	}

	return conn, nil
}

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
