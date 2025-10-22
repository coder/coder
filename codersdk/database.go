package codersdk

import "golang.org/x/xerrors"

const DatabaseNotReachable = "database not reachable"

var ErrDatabaseNotReachable = xerrors.New(DatabaseNotReachable)
