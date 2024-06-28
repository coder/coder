package tailnet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/netip"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/tailnet/proto"
)

var ipv4And6Regex = regexp.MustCompile(`(((::ffff:)?(25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}|([a-f0-9:]+:+)+[a-f0-9]+)`)

// Used to store a number of slog logger, and a logger sink for creating network telemetry events
type multiLogger struct {
	loggers []slog.Logger
}

func newMultiLogger(loggers ...slog.Logger) multiLogger {
	return multiLogger{loggers: loggers}
}

func (m multiLogger) appendLogger(logger slog.Logger) multiLogger {
	return multiLogger{loggers: append(m.loggers, logger)}
}

func (m multiLogger) Critical(ctx context.Context, msg string, fields ...any) {
	for _, i := range m.loggers {
		i.Critical(ctx, msg, fields...)
	}
}

func (m multiLogger) Debug(ctx context.Context, msg string, fields ...any) {
	for _, i := range m.loggers {
		i.Debug(ctx, msg, fields...)
	}
}

func (m multiLogger) Error(ctx context.Context, msg string, fields ...any) {
	for _, i := range m.loggers {
		i.Error(ctx, msg, fields...)
	}
}

func (m multiLogger) Fatal(ctx context.Context, msg string, fields ...any) {
	for _, i := range m.loggers {
		i.Fatal(ctx, msg, fields...)
	}
}

func (m multiLogger) Info(ctx context.Context, msg string, fields ...any) {
	for _, i := range m.loggers {
		i.Info(ctx, msg, fields...)
	}
}

func (m multiLogger) Warn(ctx context.Context, msg string, fields ...any) {
	for _, i := range m.loggers {
		i.Warn(ctx, msg, fields...)
	}
}

func (m multiLogger) Named(name string) multiLogger {
	var loggers []slog.Logger
	for _, i := range m.loggers {
		loggers = append(loggers, i.Named(name))
	}
	return multiLogger{loggers: loggers}
}

func (m multiLogger) With(fields ...slog.Field) multiLogger {
	var loggers []slog.Logger
	for _, i := range m.loggers {
		loggers = append(loggers, i.With(fields...))
	}
	return multiLogger{loggers: loggers}
}

// A logger sink that extracts (anonymized) IP addresses from logs for building
// network telemetry events
type TelemetryStore struct {
	// Always self-referential
	sink slog.Sink
	mu   sync.Mutex
	// TODO: Store only useful logs
	logs     []string
	hashSalt string
	// A cache to avoid hashing the same IP or hostname multiple times.
	hashCache map[string]string
	hashedIPs map[string]*proto.IPFields

	cleanDerpMap  *tailcfg.DERPMap
	derpMapFilter *regexp.Regexp
}

var _ slog.Sink = &TelemetryStore{}

var _ io.Writer = &TelemetryStore{}

func newTelemetryStore() (*TelemetryStore, error) {
	hashSalt, err := cryptorand.String(16)
	if err != nil {
		return nil, err
	}
	out := &TelemetryStore{
		logs:          []string{},
		hashSalt:      hashSalt,
		hashCache:     make(map[string]string),
		hashedIPs:     make(map[string]*proto.IPFields),
		derpMapFilter: regexp.MustCompile(`^$`),
	}
	out.sink = sloghuman.Sink(out)
	return out, nil
}

func (b *TelemetryStore) getStore() ([]string, map[string]*proto.IPFields, *tailcfg.DERPMap) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string{}, b.logs...), b.hashedIPs, b.cleanDerpMap.Clone()
}

// Given a DERPMap, anonymise all IPs and hostnames.
// Keep track of seen hostnames/cert names to anonymize them from future logs.
// b.mu must NOT be held.
func (b *TelemetryStore) updateDerpMap(cur *tailcfg.DERPMap) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var names []string
	cleanMap := cur.Clone()
	for _, r := range cleanMap.Regions {
		for _, n := range r.Nodes {
			escapedName := regexp.QuoteMeta(n.HostName)
			escapedCertName := regexp.QuoteMeta(n.CertName)
			names = append(names, escapedName, escapedCertName)

			ipv4, _ := b.processIPLocked(n.IPv4)
			n.IPv4 = ipv4
			ipv6, _ := b.processIPLocked(n.IPv6)
			n.IPv6 = ipv6
			stunIP, _ := b.processIPLocked(n.STUNTestIP)
			n.STUNTestIP = stunIP
			hn := b.hashAddr(n.HostName)
			n.HostName = hn
			cn := b.hashAddr(n.CertName)
			n.CertName = cn
		}
	}
	if len(names) != 0 {
		b.derpMapFilter = regexp.MustCompile((strings.Join(names, "|")))
	}
	b.cleanDerpMap = cleanMap
}

// Write implements io.Writer.
func (b *TelemetryStore) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// sloghuman writes a full log line in a single Write call with a trailing
	// newline.
	logLine := strings.TrimSuffix(string(p), "\n")

	logLineSplit := strings.SplitN(logLine, "]", 2)
	logLineAfterLevel := logLine
	if len(logLineAfterLevel) == 2 {
		logLineAfterLevel = logLineSplit[1]
	}
	// Anonymize IP addresses
	for _, match := range ipv4And6Regex.FindAllString(logLineAfterLevel, -1) {
		hash, err := b.processIPLocked(match)
		if err == nil {
			logLine = strings.ReplaceAll(logLine, match, hash)
		}
	}
	// Anonymize derp map host names
	for _, match := range b.derpMapFilter.FindAllString(logLineAfterLevel, -1) {
		hash := b.hashAddr(match)
		logLine = strings.ReplaceAll(logLine, match, hash)
	}

	b.logs = append(b.logs, logLine)
	return len(p), nil
}

// LogEntry implements slog.Sink.
func (b *TelemetryStore) LogEntry(ctx context.Context, e slog.SinkEntry) {
	// This will call (*bufferLogSink).Write
	b.sink.LogEntry(ctx, e)
}

// Sync implements slog.Sink.
func (b *TelemetryStore) Sync() {
	b.sink.Sync()
}

// processIPLocked will look up the IP in the cache, or hash and salt it and add
// to the cache. It will also add it to hashedIPs.
//
// b.mu must be held.
func (b *TelemetryStore) processIPLocked(ip string) (string, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", xerrors.Errorf("failed to parse IP %q: %w", ip, err)
	}
	version := int32(4)
	if addr.Is6() {
		version = 6
	}

	class := proto.IPFields_PUBLIC
	switch {
	case addr.IsLoopback():
		class = proto.IPFields_LOOPBACK
	case addr.IsLinkLocalUnicast():
		class = proto.IPFields_LINK_LOCAL
	case addr.IsLinkLocalMulticast():
		class = proto.IPFields_LINK_LOCAL
	case addr.IsPrivate():
		class = proto.IPFields_PRIVATE
	}

	hashStr := b.hashAddr(ip)
	b.hashedIPs[hashStr] = &proto.IPFields{
		Version: version,
		Class:   class,
	}
	return hashStr, nil
}

func (b *TelemetryStore) hashAddr(addr string) string {
	if hashStr, ok := b.hashCache[addr]; ok {
		return hashStr
	}

	hash := sha256.Sum256([]byte(b.hashSalt + addr))
	hashStr := hex.EncodeToString(hash[:])
	b.hashCache[addr] = hashStr
	return hashStr
}
