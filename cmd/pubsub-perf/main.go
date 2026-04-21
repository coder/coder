package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"

	"github.com/coder/coder/v2/coderd/database/pubsub"
)

type config struct {
	postgresURL    string
	numPubsubs     int
	numPublishers  int
	publishRateHz  float64
	messageSize    int
	duration       time.Duration
	noSubFraction  float64
	prometheusAddr string
	maxConns       int
	verbose        bool
}

func (c *config) registerFlags() {
	flag.StringVar(&c.postgresURL, "postgres-url", "",
		"PostgreSQL connection URL (required)")
	flag.IntVar(&c.numPubsubs, "num-pubsubs", 3,
		"Number of PGPubsub instances, each with an isolated connection pool")
	flag.IntVar(&c.numPublishers, "num-publishers", 10,
		"Total number of publishers (each gets a unique event channel)")
	flag.Float64Var(&c.publishRateHz, "publish-rate-hz", 10,
		"Publishes per second per publisher")
	flag.IntVar(&c.messageSize, "message-size", 128,
		"Message payload size in bytes (minimum 20)")
	flag.DurationVar(&c.duration, "duration", 30*time.Second,
		"Test duration")
	flag.Float64Var(&c.noSubFraction, "no-subscriber-fraction", 0.1,
		"Fraction of publishers with no subscribers (0.0-1.0)")
	flag.StringVar(&c.prometheusAddr, "prometheus-addr", ":6060",
		"Address for Prometheus metrics HTTP server")
	flag.IntVar(&c.maxConns, "max-conns-per-pubsub", 8,
		"Max open database connections per pubsub instance")
	flag.BoolVar(&c.verbose, "verbose", false,
		"Enable verbose (debug) logging")
}

func (c *config) validate() error {
	if c.postgresURL == "" {
		return fmt.Errorf("--postgres-url is required")
	}
	if c.numPubsubs < 1 {
		return fmt.Errorf("--num-pubsubs must be >= 1")
	}
	if c.numPublishers < 1 {
		return fmt.Errorf("--num-publishers must be >= 1")
	}
	if c.publishRateHz <= 0 {
		return fmt.Errorf("--publish-rate-hz must be > 0")
	}
	if c.messageSize < 20 {
		return fmt.Errorf("--message-size must be >= 20 to fit timestamp")
	}
	if c.noSubFraction < 0 || c.noSubFraction > 1 {
		return fmt.Errorf("--no-subscriber-fraction must be between 0.0 and 1.0")
	}
	if c.maxConns < 2 {
		// PGPubsub needs at least 1 connection for LISTEN plus 1
		// for publishing.
		return fmt.Errorf("--max-conns-per-pubsub must be >= 2")
	}
	return nil
}

// perfMetrics holds the Prometheus metrics specific to this perf test.
type perfMetrics struct {
	publishesTotal    *prometheus.CounterVec
	receivedTotal     prometheus.Counter
	latencySeconds    prometheus.Histogram
	publishErrorTotal prometheus.Counter
}

func newPerfMetrics() *perfMetrics {
	return &perfMetrics{
		publishesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pubsub_perf",
			Name:      "publishes_total",
			Help:      "Total number of messages published.",
		}, []string{"has_subscribers"}),
		receivedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "pubsub_perf",
			Name:      "received_total",
			Help:      "Total number of messages received by subscribers.",
		}),
		latencySeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "pubsub_perf",
			Name:      "latency_seconds",
			Help:      "End-to-end publish-to-subscribe latency in seconds.",
			Buckets: []float64{
				0.0001, 0.00025, 0.0005,
				0.001, 0.0025, 0.005,
				0.01, 0.025, 0.05,
				0.1, 0.25, 0.5,
				1, 2.5, 5, 10,
			},
		}),
		publishErrorTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "pubsub_perf",
			Name:      "publish_errors_total",
			Help:      "Total number of publish errors.",
		}),
	}
}

func (m *perfMetrics) register(reg prometheus.Registerer) {
	reg.MustRegister(
		m.publishesTotal,
		m.receivedTotal,
		m.latencySeconds,
		m.publishErrorTotal,
	)
}

// stats tracks summary counters using atomic operations so the final
// report does not depend on a Prometheus scrape.
type stats struct {
	publishes     atomic.Int64
	receives      atomic.Int64
	publishErrors atomic.Int64
	latencySumNs  atomic.Int64
	latencyCount  atomic.Int64
}

func main() {
	var cfg config
	cfg.registerFlags()
	flag.Parse()

	if err := cfg.validate(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logLevel := slog.LevelWarn
	if cfg.verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.Make(sloghuman.Sink(os.Stderr)).Leveled(logLevel)

	// Derived counts.
	numNoSub := int(float64(cfg.numPublishers) * cfg.noSubFraction)
	numWithSub := cfg.numPublishers - numNoSub
	totalSubCount := numWithSub * cfg.numPubsubs

	fmt.Println("=== pubsub-perf ===")
	fmt.Printf("  Pubsub instances:       %d\n", cfg.numPubsubs)
	fmt.Printf("  Publishers (total):      %d\n", cfg.numPublishers)
	fmt.Printf("    with subscribers:      %d\n", numWithSub)
	fmt.Printf("    without subscribers:   %d\n", numNoSub)
	fmt.Printf("  Subscribers (total):     %d\n", totalSubCount)
	fmt.Printf("  Publish rate per pub:    %.1f Hz\n", cfg.publishRateHz)
	fmt.Printf("  Aggregate publish rate:  %.1f Hz\n",
		cfg.publishRateHz*float64(cfg.numPublishers))
	fmt.Printf("  Message size:            %d bytes\n", cfg.messageSize)
	fmt.Printf("  Duration:                %s\n", cfg.duration)
	fmt.Printf("  Max DB conns per pubsub: %d\n", cfg.maxConns)
	fmt.Printf("  Prometheus:              %s\n", cfg.prometheusAddr)
	fmt.Println()
	// ---- Database ping -----------------------------------------------------
	pingLatency, err := measurePingLatency(cfg.postgresURL)
	if err != nil {
		return fmt.Errorf("database ping: %w", err)
	}
	fmt.Printf("Database ping latency:     %s\n\n", pingLatency)


	// ---- Prometheus setup ------------------------------------------------
	pm := newPerfMetrics()
	reg := prometheus.NewRegistry()
	pm.register(reg)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	httpSrv := &http.Server{
		Addr:              cfg.prometheusAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil &&
			err != http.ErrServerClosed {
			logger.Error(ctx, "prometheus http server error",
				slog.Error(err))
		}
	}()
	defer httpSrv.Close()

	// ---- Pubsub instances ------------------------------------------------
	type psEntry struct {
		ps pubsub.Pubsub
		db *sql.DB
	}
	instances := make([]psEntry, cfg.numPubsubs)
	for i := range cfg.numPubsubs {
		db, err := sql.Open("postgres", cfg.postgresURL)
		if err != nil {
			return fmt.Errorf("sql.Open for pubsub %d: %w", i, err)
		}
		db.SetMaxOpenConns(cfg.maxConns)
		db.SetMaxIdleConns(cfg.maxConns)

		ps, err := pubsub.New(ctx,
			logger.Named(fmt.Sprintf("ps%d", i)), db, cfg.postgresURL)
		if err != nil {
			// Close already-created instances.
			for j := i - 1; j >= 0; j-- {
				instances[j].ps.Close()
				instances[j].db.Close()
			}
			db.Close()
			return fmt.Errorf("pubsub.New for instance %d: %w", i, err)
		}
		instances[i] = psEntry{ps: ps, db: db}
	}
	defer func() {
		for i := len(instances) - 1; i >= 0; i-- {
			instances[i].ps.Close()
			instances[i].db.Close()
		}
	}()
	fmt.Printf("Created %d pubsub instances.\n", cfg.numPubsubs)

	// ---- Subscriptions ---------------------------------------------------
	// Publishers 0..numNoSub-1 have NO subscribers.
	// Publishers numNoSub..numPublishers-1 have a subscriber on every
	// pubsub instance (including the publishing instance), which
	// matches how Coder Server replicas all LISTEN on shared channels.
	var st stats
	var unsubs []func()
	for i := numNoSub; i < cfg.numPublishers; i++ {
		event := fmt.Sprintf("perf-event-%d", i)
		for j := range cfg.numPubsubs {
			fn, err := instances[j].ps.Subscribe(event,
				makeSubscriberCallback(pm, &st))
			if err != nil {
				// Clean up already-registered subscriptions.
				for _, u := range unsubs {
					u()
				}
				return fmt.Errorf(
					"subscribe ps%d to %q: %w", j, event, err)
			}
			unsubs = append(unsubs, fn)
		}
	}
	defer func() {
		for _, u := range unsubs {
			u()
		}
	}()
	fmt.Printf("Subscriptions ready (%d). Starting publishers...\n\n",
		len(unsubs))

	// ---- Publishers ------------------------------------------------------
	publishCtx, publishCancel := context.WithCancel(ctx)
	defer publishCancel()

	var wg sync.WaitGroup
	for i := range cfg.numPublishers {
		hasSub := i >= numNoSub
		event := fmt.Sprintf("perf-event-%d", i)
		psIdx := i % cfg.numPubsubs
		ps := instances[psIdx].ps

		hasSubLabel := "false"
		if hasSub {
			hasSubLabel = "true"
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			runPublisher(publishCtx, ps, event, hasSubLabel,
				cfg, pm, &st)
		}()
	}

	// ---- Wait for duration or interrupt ----------------------------------
	timer := time.NewTimer(cfg.duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		fmt.Println("Duration elapsed, stopping publishers...")
	case <-ctx.Done():
		fmt.Println("\nInterrupted, stopping publishers...")
	}
	publishCancel()
	wg.Wait()

	// Allow in-flight notifications to be delivered. This is a
	// deliberate drain period, not a flaky-test workaround.
	drainCtx, drainCancel := context.WithTimeout(
		context.Background(), 2*time.Second)
	defer drainCancel()
	<-drainCtx.Done()

	printSummary(cfg, &st)
	return nil
}

// makeSubscriberCallback returns a pubsub.Listener that records latency
// and receive counts.
func makeSubscriberCallback(pm *perfMetrics, st *stats) pubsub.Listener {
	return func(_ context.Context, msg []byte) {
		recvNano := time.Now().UnixNano()
		sendNano, err := parseTimestamp(msg)
		if err != nil {
			return
		}
		latencyNs := recvNano - sendNano
		if latencyNs < 0 {
			latencyNs = 0
		}
		latencySec := float64(latencyNs) / float64(time.Second)

		pm.latencySeconds.Observe(latencySec)
		pm.receivedTotal.Inc()

		st.receives.Add(1)
		st.latencySumNs.Add(latencyNs)
		st.latencyCount.Add(1)
	}
}

// runPublisher publishes messages at a fixed rate until ctx is cancelled.
func runPublisher(
	ctx context.Context,
	ps pubsub.Pubsub,
	event string,
	hasSubLabel string,
	cfg config,
	pm *perfMetrics,
	st *stats,
) {
	interval := time.Duration(
		float64(time.Second) / cfg.publishRateHz)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			msg := buildMessage(cfg.messageSize)
			if err := ps.Publish(event, msg); err != nil {
				pm.publishErrorTotal.Inc()
				st.publishErrors.Add(1)
				continue
			}
			pm.publishesTotal.WithLabelValues(hasSubLabel).Inc()
			st.publishes.Add(1)
		}
	}
}

// buildMessage creates a text payload with the current nanosecond
// timestamp as a decimal prefix, separated by '|', padded to size.
//
// Format: "<unix_nano>|<padding>"
func buildMessage(size int) []byte {
	ts := strconv.FormatInt(time.Now().UnixNano(), 10)
	buf := make([]byte, size)
	n := copy(buf, ts)
	if n < size {
		buf[n] = '|'
		n++
	}
	for i := n; i < size; i++ {
		buf[i] = 'x'
	}
	return buf
}

// parseTimestamp extracts the nanosecond timestamp from a message
// built by buildMessage.
func parseTimestamp(msg []byte) (int64, error) {
	s := string(msg)
	if sep := strings.IndexByte(s, '|'); sep >= 0 {
		s = s[:sep]
	}
	return strconv.ParseInt(s, 10, 64)
}

func printSummary(cfg config, st *stats) {
	pub := st.publishes.Load()
	recv := st.receives.Load()
	errs := st.publishErrors.Load()
	latN := st.latencyCount.Load()
	latSum := st.latencySumNs.Load()

	fmt.Println()
	fmt.Println("=== Summary ===")
	fmt.Printf("  Duration:          %s\n", cfg.duration)
	fmt.Printf("  Total publishes:   %d\n", pub)
	fmt.Printf("  Publish errors:    %d\n", errs)
	fmt.Printf("  Total receives:    %d\n", recv)
	if pub > 0 {
		fmt.Printf("  Publish throughput: %.1f msg/s\n",
			float64(pub)/cfg.duration.Seconds())
	}
	if recv > 0 {
		fmt.Printf("  Receive throughput: %.1f msg/s\n",
			float64(recv)/cfg.duration.Seconds())
	}
	if latN > 0 {
		avgMs := float64(latSum) / float64(latN) / 1e6
		fmt.Printf("  Avg latency:       %.3f ms\n", avgMs)
	}
	fmt.Println()
	fmt.Println("Detailed latency percentiles available via Prometheus")
	fmt.Printf("  GET %s/metrics\n", cfg.prometheusAddr)
}

// measurePingLatency opens a throwaway connection, pings the database
// several times, and returns the median round-trip duration.
func measurePingLatency(postgresURL string) (time.Duration, error) {
	db, err := sql.Open("postgres", postgresURL)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	const rounds = 5
	samples := make([]time.Duration, 0, rounds)
	for range rounds {
		start := time.Now()
		if err := db.Ping(); err != nil {
			return 0, err
		}
		samples = append(samples, time.Since(start))
	}

	// Return the median.
	slices.Sort(samples)
	return samples[rounds/2], nil
}
