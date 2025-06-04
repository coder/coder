package database

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/coder/coder/v2/coderd/util/slice"
)

// PGLock docs see: https://www.postgresql.org/docs/current/view-pg-locks.html#VIEW-PG-LOCKS
type PGLock struct {
	// LockType see: https://www.postgresql.org/docs/current/monitoring-stats.html#WAIT-EVENT-LOCK-TABLE
	LockType           *string    `db:"locktype"`
	Database           *string    `db:"database"` // oid
	Relation           *string    `db:"relation"` // oid
	RelationName       *string    `db:"relation_name"`
	Page               *int       `db:"page"`
	Tuple              *int       `db:"tuple"`
	VirtualXID         *string    `db:"virtualxid"`
	TransactionID      *string    `db:"transactionid"` // xid
	ClassID            *string    `db:"classid"`       // oid
	ObjID              *string    `db:"objid"`         // oid
	ObjSubID           *int       `db:"objsubid"`
	VirtualTransaction *string    `db:"virtualtransaction"`
	PID                int        `db:"pid"`
	Mode               *string    `db:"mode"`
	Granted            bool       `db:"granted"`
	FastPath           *bool      `db:"fastpath"`
	WaitStart          *time.Time `db:"waitstart"`
}

func (l PGLock) Equal(b PGLock) bool {
	// Lazy, but hope this works
	return reflect.DeepEqual(l, b)
}

func (l PGLock) String() string {
	granted := "granted"
	if !l.Granted {
		granted = "waiting"
	}
	var details string
	switch safeString(l.LockType) {
	case "relation":
		details = ""
	case "page":
		details = fmt.Sprintf("page=%d", *l.Page)
	case "tuple":
		details = fmt.Sprintf("page=%d tuple=%d", *l.Page, *l.Tuple)
	case "virtualxid":
		details = "waiting to acquire virtual tx id lock"
	default:
		details = "???"
	}
	return fmt.Sprintf("%d-%5s [%s] %s/%s/%s: %s",
		l.PID,
		safeString(l.TransactionID),
		granted,
		safeString(l.RelationName),
		safeString(l.LockType),
		safeString(l.Mode),
		details,
	)
}

// PGLocks returns a list of all locks in the database currently in use.
func (q *sqlQuerier) PGLocks(ctx context.Context) (PGLocks, error) {
	rows, err := q.sdb.QueryContext(ctx, `
	SELECT
		relation::regclass AS relation_name,
	    *
	FROM pg_locks;
	`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var locks []PGLock
	err = sqlx.StructScan(rows, &locks)
	if err != nil {
		return nil, err
	}

	return locks, err
}

type PGLocks []PGLock

func (l PGLocks) String() string {
	// Try to group things together by relation name.
	sort.Slice(l, func(i, j int) bool {
		return safeString(l[i].RelationName) < safeString(l[j].RelationName)
	})

	var out strings.Builder
	for i, lock := range l {
		if i != 0 {
			_, _ = out.WriteString("\n")
		}
		_, _ = out.WriteString(lock.String())
	}
	return out.String()
}

// Difference returns the difference between two sets of locks.
// This is helpful to determine what changed between the two sets.
func (l PGLocks) Difference(to PGLocks) (newVal PGLocks, removed PGLocks) {
	return slice.SymmetricDifferenceFunc(l, to, func(a, b PGLock) bool {
		return a.Equal(b)
	})
}
