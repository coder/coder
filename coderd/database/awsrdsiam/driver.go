package awsrdsiam

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

type awsRdsIamDriver struct {
	parent driver.Driver
	cfg    aws.Config
}

var _ driver.Driver = &awsRdsIamDriver{}

// Register initializes and registers our aws rds iam wrapped database driver.
func Register(ctx context.Context, parentName string) (string, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", err
	}

	db, err := sql.Open(parentName, "")
	if err != nil {
		return "", err
	}

	// create a new aws rds iam driver
	d := newDriver(db.Driver(), cfg)
	name := fmt.Sprintf("%s-awsrdsiam", parentName)
	sql.Register(fmt.Sprintf("%s-awsrdsiam", parentName), d)

	return name, nil
}

// newDriver will create a new *AwsRdsIamDriver using the environment aws session.
func newDriver(parentDriver driver.Driver, cfg aws.Config) *awsRdsIamDriver {
	return &awsRdsIamDriver{
		parent: parentDriver,
		cfg:    cfg,
	}
}

// Open creates a new connection to the database using the provided name.
func (d *awsRdsIamDriver) Open(name string) (driver.Conn, error) {
	// set password with signed aws authentication token for the rds instance
	nURL, err := getAuthenticatedURL(d.cfg, name)
	if err != nil {
		return nil, xerrors.Errorf("assigning authentication token to url: %w", err)
	}

	// make connection
	conn, err := d.parent.Open(nURL)
	if err != nil {
		return nil, xerrors.Errorf("opening connection: %w", err)
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
