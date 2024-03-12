package awsrdsiam

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds/rdsutils"
	"golang.org/x/xerrors"
)

type AwsRdsIamDriver struct {
	parent driver.Driver
	sess   *session.Session
	dbURL  string
}

var _ driver.Driver = &AwsRdsIamDriver{}

// Register initializes and registers our aws rds iam wrapped database driver.
func Register(parentName string, dbURL string) (string, error) {
	sess, err := session.NewSession()
	if err != nil {
		return "", xerrors.Errorf("creating aws session: %w", err)
	}

	db, err := sql.Open(parentName, "")
	if err != nil {
		return "", err
	}

	// create a new aws rds iam driver
	d := newDriver(db.Driver(), sess, dbURL)
	name := fmt.Sprintf("%s-awsrdsiam", parentName)
	sql.Register(fmt.Sprintf("%s-awsrdsiam", parentName), d)

	return name, nil
}

// newDriver will create a new *AwsRdsIamDriver using the environment aws session.
func newDriver(parentDriver driver.Driver, sess *session.Session, dbURL string) *AwsRdsIamDriver {
	return &AwsRdsIamDriver{
		parent: parentDriver,
		sess:   sess,
		dbURL:  dbURL,
	}
}

// Open creates a new connection to the database using the provided name.
func (d *AwsRdsIamDriver) Open(name string) (driver.Conn, error) {
	// set password with signed aws authentication token for the rds instance
	nURL, err := getAuthenticatedURL(d.sess, name)
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

func getAuthenticatedURL(sess *session.Session, dbURL string) (string, error) {
	nURL, err := url.Parse(dbURL)
	if err != nil {
		return "", xerrors.Errorf("parsing dbURL: %w", err)
	}

	// generate a new rds session auth tokenized URL
	rdsEndpoint := fmt.Sprintf("%s:%s", nURL.Hostname(), nURL.Port())
	token, err := rdsutils.BuildAuthToken(rdsEndpoint, *sess.Config.Region, nURL.User.Username(), sess.Config.Credentials)
	if err != nil {
		return "", xerrors.Errorf("building rds auth token: %w", err)
	}
	// set token as user password
	nURL.User = url.UserPassword(nURL.User.Username(), token)

	return nURL.String(), nil
}
