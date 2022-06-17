package databasefake

import (
	"bytes"
	"runtime"
	"strconv"
	"sync"
)

// reentrantLock is a lock that can be locked multiple times from the same goroutine.
//
// Go maintainers insist that this is a Bad Idea and refuse to implement it in the standard library, so let's talk about
// why we're doing it here.
//
// We want to support locking the fake database for the duration of a transaction, so that other goroutines cannot see
// uncommitted transactions.  However, we also need to lock the database during queries that are not explicitly in a
// transaction.  When a goroutine executing a transaction calls a query, it is already holding the lock, so attempting
// to lock a standard mutex again will deadlock.  A reentrant lock neatly solves this problem.
//
// The argument I've heard around why reentrant locks are a Bad Idea points out that it indicates a problem with your
// interface, because some methods must leave the database in an inconsistent state.  That criticism applies here
// because the whole reason we need transactions is sometimes individual queries leave the database in an inconsistent
// state. However valid the criticism, the assumption that it becomes the most important factor in all cases is flawed
// and, frankly, patronizing.
//
// Here we do not have the luxury of reinventing the interface, because this fake database is attempting to
// emulate another piece of software which does have this interface: postgres. Basically, the logic that enforces the
// consistency of the database resides at a higher layer than we are emulating.
//
// Some alternatives considered, but rejected:
//
//  1. create an explicit transaction type, which are serialized to a channel and then processed in order.
//     * requires implementing each query function twice, once wrapping it in a transaction, and once doing the real
//       work
//     * cannot support recursive transactions
//  2. store whether we're in a transaction in the Context passed to the queries.
//     * changes InTx(func(store) error) -> InTx(ctx, func(ctx2, store) error).  Inside the transaction function,
//       callers **must use** ctx2 to query.  Use of other contexts, like ctx, will deadlock.  Adding this tripmine
//       to every use of transactions seems like a recipe for bugs that are hard to diagnose (and only show up with the
//       fake database).
type reentrantLock struct {
	c      *sync.Cond
	holder uint64
	n      uint64
}

// getGID returns the goroutine ID.
//
// From https://blog.sgmansfield.com/2015/12/goroutine-ids/
func getGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

func newReentrantLock() sync.Locker {
	return &reentrantLock{
		c:      sync.NewCond(&sync.Mutex{}),
		holder: 0,
		n:      0,
	}
}

func (r *reentrantLock) Lock() {
	gID := getGID()
	r.c.L.Lock()
	defer r.c.L.Unlock()
	for {
		if r.holder == 0 {
			// not held by any goroutine
			r.holder = gID
			break
		}
		if r.holder == gID {
			// held by us
			break
		}
		r.c.Wait()
	}
	r.n++
}

func (r *reentrantLock) Unlock() {
	gID := getGID()
	r.c.L.Lock()
	defer r.c.L.Unlock()
	if r.holder != gID {
		panic("unlocked without holding lock")
	}
	r.n--
	if r.n == 0 {
		r.holder = 0
		r.c.Signal()
	}
}
