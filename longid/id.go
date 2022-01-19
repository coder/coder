package longid

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"
)

// bit counts.
const (
	TimeBits        = 32
	IncrementorBits = 10

	RandomBits = 70
	HostIDBits = 8

	// amount of random bits in each uint64
	RandomBits1 = 64 - (TimeBits + IncrementorBits)
	RandomBits2 = RandomBits - RandomBits1

	HostMask = 0x00000000000000FF
)

var (
	inc    uint32
	hostID int64
)

func init() {
	rand.Seed(time.Now().UnixNano())

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(hostname))
	hostID = (int64(hash.Sum64())) & HostMask
	inc = rand.Uint32()
}

// HostID returns the host ID for the current machine.
func HostID() int64 {
	return hostID
}

// ID describes a 128 bit ID
type ID [16]byte

// parse errors
var (
	ErrWrongSize = xerrors.New("id in string form should be exactly 33 bytes")
)

// FromSlice converts a slice into an ID.
func FromSlice(b []byte) ID {
	var l ID
	copy(l[:], b)
	return l
}

func part1() int64 {
	seconds := time.Now().Unix()

	// place time portion properly
	time := seconds << (64 - TimeBits)

	i := atomic.AddUint32(&inc, 1)

	// reset incrementor if it's too big
	atomic.CompareAndSwapUint32(&inc, ((1 << IncrementorBits) - 1), 0)

	i <<= (RandomBits1)

	var randBuf [4]byte
	_, _ = rand.Read(randBuf[:])

	rand := (binary.BigEndian.Uint32(randBuf[:]) >> (32 - RandomBits1))

	return time + int64(i) + int64(rand)
}

func part2() int64 {
	var randBuf [8]byte
	_, _ = rand.Read(randBuf[:])
	rand := binary.BigEndian.Uint64(randBuf[:]) << HostIDBits
	// fmt.Printf("%x\n", rand)

	return int64(rand) + hostID
}

// New generates a long ID.
func New() ID {
	var id ID
	binary.BigEndian.PutUint64(id[:8], uint64(part1()))
	binary.BigEndian.PutUint64(id[8:], uint64(part2()))
	return id
}

// Bytes returns a byte slice from l.
func (l ID) Bytes() []byte {
	return l[:]
}

// CreatedAt returns the time the ID was created at.
func (l ID) CreatedAt() time.Time {
	epoch := (time.Now().Unix() >> (TimeBits)) << (TimeBits)

	ts := binary.BigEndian.Uint64(l[:8]) >> (64 - TimeBits)

	// fmt.Printf("%064b\n", epoch)
	// fmt.Printf("%064b\n", ts)

	return time.Unix(epoch+int64(ts), 0)
}

// String returns the text representation of l
func (l ID) String() string {
	return fmt.Sprintf("%08x-%024x", l[:4], l[4:])
}

// MarshalText marshals l
func (l ID) MarshalText() ([]byte, error) {
	return []byte(l.String()), nil
}

// UnmarshalText parses b
func (l *ID) UnmarshalText(b []byte) error {
	ll, err := Parse(string(b))
	if err != nil {
		return err
	}
	copy(l[:], ll[:])
	return nil
}

// MarshalJSON marshals l
func (l ID) MarshalJSON() ([]byte, error) {
	return []byte("\"" + l.String() + "\""), nil
}

// UnmarshalJSON parses b
func (l *ID) UnmarshalJSON(b []byte) error {
	return l.UnmarshalText(bytes.Trim(b, "\""))
}

var _ = driver.Valuer(New())
var _ = sql.Scanner(&ID{})

func (l ID) Value() (driver.Value, error) {
	return l.Bytes(), nil
}

func (l *ID) Scan(v interface{}) error {
	b, ok := v.([]byte)
	if !ok {
		return xerrors.New("can only scan binary types")
	}
	if len(b) != 16 {
		return xerrors.New("must be 16 bytes")
	}
	copy(l[:], b)
	return nil
}

// Parse parses the String() representation of a Long
func Parse(l string) (ID, error) {
	var (
		id  ID
		err error
	)
	if len(l) != 33 {
		return id, ErrWrongSize
	}

	p1, err := hex.DecodeString(l[:8])
	if err != nil {
		return id, xerrors.Errorf("failed to decode short portion: %w", err)
	}

	p2, err := hex.DecodeString(l[9:])
	if err != nil {
		return id, xerrors.Errorf("failed to decode rand portion: %w", err)
	}

	copy(id[:4], p1)
	copy(id[4:], p2)

	return id, nil
}

// TimeReset the current bounds of
// validity for timestamps extracted from longs
func TimeReset() (last time.Time, next time.Time) {
	const lastStr = "00000000-00680e087d8fff20a11d24e6"
	const nextStr = "ffffffff-00680e087d8fff20a11d24e6"
	l, _ := Parse(lastStr)
	last = l.CreatedAt()

	l, _ = Parse(nextStr)
	next = l.CreatedAt()

	return
}
