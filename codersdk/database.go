package codersdk

import "golang.org/x/xerrors"

const DatabaseNotReachable = "database not reachable"

var ErrDatabaseNotReachable = xerrors.New(DatabaseNotReachable)

// @typescript-ignore DefaultMaxConns
// @typescript-ignore DefaultIdleConns
const (
	DefaultMaxConns = 10
	// DefaultIdleConns is set to 3 idle connections because lower values end up
	// creating a lot of connection churn. Since each connection uses about
	// 10MB of memory, we're allocating 30MB to Postgres connections per
	// replica, but is better than causing Postgres to spawn a thread 15-20
	// times/sec. PGBouncer's transaction pooling is not the greatest so
	// it's not optimal for us to deploy.
	//
	// This was set to 10 before we started doing HA deployments, but 3 was
	// later determined to be a better middle ground as to not use up all
	// of PGs default connection limit while simultaneously avoiding a lot
	// of connection churn.
	DefaultIdleConns = 3
)
