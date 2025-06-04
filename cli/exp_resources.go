package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/PaesslerAG/jsonpath"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/exp/maps"
	"golang.org/x/xerrors"
	kresource "k8s.io/apimachinery/pkg/api/resource"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) expResourcesCmd() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "resources",
		Short: "Internal commands for testing and experimentation with resources.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.dumpBuildInfoCmd(),
			r.trackUsageCmd(),
		},
	}
	return cmd
}

func (r *RootCmd) dumpBuildInfoCmd() *serpent.Command {
	var (
		postgresURL string
		from        string
		to          string
		validate    bool
	)
	cmd := &serpent.Command{
		Use:   "dump-build-info <outfile.csv>",
		Short: "Dump all workspace builds information to CSV.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()
			logger := slog.Make(sloghuman.Sink(i.Stderr)).Named("dump_build_info")
			if r.verbose {
				logger = logger.Leveled(slog.LevelDebug)
			}

			sqlDB, err := sql.Open("postgres", postgresURL)
			if err != nil {
				return xerrors.Errorf("connect to database: %w", err)
			}
			defer sqlDB.Close()
			if err := sqlDB.PingContext(ctx); err != nil {
				return xerrors.Errorf("ping database: %w", err)
			}

			fromTime, toTime := codersdk.NullTime{}, codersdk.NullTime{}
			if from != "" {
				fromTime.Time, err = time.Parse(time.RFC3339Nano, from)
				if err != nil {
					return xerrors.Errorf("parse from time: %w", err)
				}
				fromTime.Valid = true
			}
			if to != "" {
				toTime.Time, err = time.Parse(time.RFC3339Nano, to)
				if err != nil {
					return xerrors.Errorf("parse to time: %w", err)
				}
				toTime.Valid = true
			}

			outfile, err := os.OpenFile(i.Args[0], os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				return xerrors.Errorf("open output file: %w", err)
			}

			builds, err := listBuilds(ctx, logger, sqlDB, fromTime, toTime)
			if err != nil {
				return xerrors.Errorf("list workspace builds: %w", err)
			}
			if len(builds) == 0 {
				cliui.Info(i.Stdout, "No workspace builds found")
				return nil
			}

			ww := WorkspaceBuildInfoCSVWriter{w: outfile}
			if err := ww.Write(builds...); err != nil {
				return xerrors.Errorf("write workspace builds to CSV: %w", err)
			}
			logger.Debug(ctx, "dumped workspace builds information")

			if validate {
				cliui.Info(i.Stderr, "Validating output...")
				// read the info back to verify it was written correctly
				infile, err := os.Open(i.Args[0])
				if err != nil {
					return xerrors.Errorf("open output file for reading: %w", err)
				}
				defer infile.Close()
				wr := WorkspaceBuildInfoCSVReader{R: infile}
				readBuilds, err := wr.Read()
				if err != nil {
					return xerrors.Errorf("read workspace builds from CSV: %w", err)
				}
				if len(readBuilds) != len(builds) {
					return xerrors.Errorf("expected %d builds, got %d", len(builds), len(readBuilds))
				}
				for idx, build := range readBuilds {
					if diff := cmp.Diff(builds[idx], build); diff != "" {
						cliui.Errorf(i.Stderr, "Mismatch in workspace build information at index %d:\n%s", idx, diff)
						return nil
					}
				}
			}
			return nil
		},
		Options: []serpent.Option{
			{
				Name:        "postgres-url",
				Description: "Postgres connection URL.",
				Flag:        "postgres-url",
				Env:         "CODER_PG_CONNECTION_URL",
				Value:       serpent.StringOf(&postgresURL),
				Required:    true,
			},
			{
				Name:        "from",
				Description: "Start time for the query, in RFC3339 format.",
				Flag:        "from",
				Required:    false,
				Value:       serpent.StringOf(&from),
			},
			{
				Name:        "to",
				Description: "End time for the query, in RFC3339 format.",
				Flag:        "to",
				Required:    false,
				Value:       serpent.StringOf(&to),
			},
			{
				Name:        "validate",
				Description: "Validate the output by reading it back and comparing it to the original data.",
				Flag:        "validate",
				Default:     "false",
				Value:       serpent.BoolOf(&validate),
			},
		},
	}
	return cmd
}

func (r *RootCmd) trackUsageCmd() *serpent.Command {
	var (
		destURL string
	)
	cmd := &serpent.Command{
		Use:   "track-usage <input.csv>",
		Short: "Given a CSV export, track resource usage by workspace builds.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()
			logger := slog.Make(sloghuman.Sink(i.Stderr)).Named("track_usage")
			if r.verbose {
				logger = logger.Leveled(slog.LevelDebug)
			}

			infile, err := os.Open(i.Args[0])
			if err != nil {
				return xerrors.Errorf("open input file: %w", err)
			}
			defer infile.Close()

			eventWriter := stdoutEventWriter(i.Stdout)
			if destURL != "" {
				logger.Debug(ctx, "using destination database for resource events", slog.F("dest_url", destURL))
				sqlDB, err := sql.Open("postgres", destURL)
				if err != nil {
					return err
				}

				defer sqlDB.Close()
				if err := sqlDB.PingContext(ctx); err != nil {
					return xerrors.Errorf("ping src database: %w", err)
				}

				eventWriter = sqlEventWriter(logger, sqlDB)
			}

			wr := WorkspaceBuildInfoCSVReader{R: infile, log: logger.Named("csv_reader")}
			builds, err := wr.Read()
			if err != nil {
				return xerrors.Errorf("read workspace build info from CSV: %w", err)
			}
			if len(builds) == 0 {
				cliui.Info(i.Stderr, "No resources detected")
				return nil
			}
			cliui.Infof(i.Stderr, "Tracking resources for %d workspace builds\n", len(builds))

			log := slog.Make(sloghuman.Sink(i.Stderr))
			if r.verbose {
				log = log.Leveled(slog.LevelDebug)
			}
			tracker := make(ResourceUsageTracker)
			allEvents := make([]ResourceUsageEvent, 0)
			for _, build := range builds {
				if foundEvents, err := tracker.Track(i.Context(), log, build); err != nil {
					return xerrors.Errorf("track resources for build %s: %w", build.WorkspaceBuildID, err)
				} else {
					allEvents = append(allEvents, foundEvents...)
				}
			}
			cliui.Infof(i.Stderr, "Tracked %d resource usage events for %d builds\n", len(allEvents), len(builds))
			if err := eventWriter(i.Context(), allEvents...); err != nil {
				return xerrors.Errorf("write resource usage events: %w", err)
			}
			return nil
		},
		Hidden: true,
		Options: []serpent.Option{
			{
				Name:        "destination-url",
				Description: "Destination URL for the output. Defaults to stdout.",
				Flag:        "dest-url",
				Default:     "",
				Value:       serpent.StringOf(&destURL),
			},
		},
	}
	return cmd
}

type intermediateTrackedResourceUsage struct {
	Start             time.Time
	UserID            uuid.UUID
	UserName          string
	WorkspaceID       uuid.UUID
	WorkspaceName     string
	TemplateVersionID uuid.UUID
	TemplateVersion   string
	TemplateID        uuid.UUID
	TemplateName      string
	ResourceID        string
	ResourceType      string
	ResourceName      string
	ResourceUnit      string
	ResourceQuantity  decimal.Decimal
	RawAttributes     string // must be stored as a JSON string to be hashable
}

func (i intermediateTrackedResourceUsage) ToEvent(finished time.Time) ResourceUsageEvent {
	// Convert the raw attributes JSON string into a map.
	var tmp map[string]any
	if err := json.Unmarshal([]byte(i.RawAttributes), &tmp); err != nil {
		// If we can't unmarshal the attributes, we just use an empty map.
		tmp = make(map[string]any)
	}
	attributes := make(map[string]string)
	// Extract the relevant attributes from the resource based on the resource
	// type.
	if extractor, found := defaultResourceUsageExtractors[i.ResourceType]; found {
		for _, e := range extractor {
			for attrName, attrPath := range e.AttributePaths {
				rawAttrVal, err := jsonpath.Get(attrPath, tmp)
				if err != nil {
					continue
				}
				attrVal, ok := rawAttrVal.(string)
				if !ok {
					continue
				}
				attributes[attrName] = attrVal
			}
		}
	}

	return ResourceUsageEvent{
		Time:              finished,
		UserID:            i.UserID,
		UserName:          i.UserName,
		WorkspaceID:       i.WorkspaceID,
		WorkspaceName:     i.WorkspaceName,
		TemplateVersionID: i.TemplateVersionID,
		TemplateVersion:   i.TemplateVersion,
		TemplateID:        i.TemplateID,
		TemplateName:      i.TemplateName,
		ResourceID:        i.ResourceID,
		ResourceType:      i.ResourceType,
		ResourceName:      i.ResourceName,
		ResourceUnit:      i.ResourceUnit,
		ResourceQuantity:  i.ResourceQuantity,
		Attributes:        attributes,
		DurationSeconds:   decimal.NewFromFloat(finished.Sub(i.Start).Seconds()),
	}
}

func convertWorkspaceBuildInfoToIntermediateTrackedResourceUsage(ctx context.Context, log slog.Logger, lbr WorkspaceBuildInfo) ([]intermediateTrackedResourceUsage, error) {
	var state tfstate
	br := bytes.NewReader(lbr.WorkspaceBuildState)
	if err := json.NewDecoder(br).Decode(&state); err != nil {
		if errors.Is(err, io.EOF) {
			log.Warn(ctx, "empty state, assuming no resources")
		} else {
			return nil, xerrors.Errorf("unmarshal workspace build state: %w", err)
		}
	}

	if lbr.JobCompletedAt.IsZero() {
		return []intermediateTrackedResourceUsage{}, nil
	}

	ret := make([]intermediateTrackedResourceUsage, 0)
	for _, res := range state.Resources {
		if res.Mode != "managed" {
			continue // We only care about managed resources.
		}

		if strings.HasPrefix(res.Type, "coder_") {
			continue // Ignore all Coder resources.
		}

		for _, instance := range res.Instances {
			instanceID, err := instance.ID()
			if err != nil {
				log.Debug(ctx, "failed to get resource instance ID", slog.F("resource_type", res.Type), slog.F("resource_name", res.Name), slog.Error(err))
				continue
			}
			if instanceID == "" {
				log.Debug(ctx, "skipping resource with no ID", slog.F("resource_type", res.Type), slog.F("resource_name", res.Name))
				continue
			}

			// Attempt to extract resource usage quantities using the default
			// extractors.
			var quantities []resourceUsageQuantity
			if qes, found := defaultResourceUsageExtractors[res.Type]; found {
				log.Debug(ctx, "extracted resource quantities", slog.F("count", len(qes)))
				for _, qe := range qes {
					q, err := qe.Extract(instance)
					if err != nil {
						log.Debug(ctx, "failed to extract resource usage", slog.F("resource_type", res.Type), slog.F("resource_name", res.Name), slog.Error(err))
						continue
					}
					quantities = append(quantities, q)
				}
			}

			if len(quantities) == 0 {
				// If no quantities were found, we default to a single unit of
				// usage.
				quantities = append(quantities, resourceUsageQuantity{
					Unit:       "unit",
					Quantity:   decimal.NewFromInt(1),
					Attributes: make(map[string]string),
				})
			}

			// Convert the instance to a JSON string to store as raw attributes.
			rawAttributes, err := json.Marshal(instance.Attributes)
			if err != nil {
				log.Debug(ctx, "failed to marshal resource attributes", slog.F("resource_type", res.Type), slog.F("resource_name", res.Name), slog.Error(err))
				rawAttributes = []byte("{}") // Fallback to empty JSON object if we can't marshal.
			}

			for _, q := range quantities {
				ret = append(ret, intermediateTrackedResourceUsage{
					Start:             lbr.JobStartedAt.UTC(),
					UserID:            lbr.UserID,
					UserName:          lbr.UserName,
					WorkspaceID:       lbr.WorkspaceID,
					WorkspaceName:     lbr.WorkspaceName,
					TemplateVersionID: lbr.TemplateVersionID,
					TemplateVersion:   lbr.TemplateVersion,
					TemplateID:        lbr.TemplateID,
					TemplateName:      lbr.TemplateName,
					ResourceID:        instanceID,
					ResourceType:      res.Type,
					ResourceName:      res.Name,
					ResourceUnit:      q.Unit,
					ResourceQuantity:  q.Quantity,
					RawAttributes:     string(rawAttributes),
				})
			}
		}
	}
	return ret, nil
}

// ResourceUsageTracker accumulates resource usage times for workspaces.
// It is fundamentally a map of workspace IDs to a map of tracked resource usages.
type ResourceUsageTracker map[uuid.UUID]map[intermediateTrackedResourceUsage]struct{}

func (r ResourceUsageTracker) Track(ctx context.Context, log slog.Logger, lbr WorkspaceBuildInfo) ([]ResourceUsageEvent, error) {
	log = log.With(
		slog.F("workspace_id", lbr.WorkspaceID),
		slog.F("workspace_name", lbr.WorkspaceName),
		slog.F("user_id", lbr.UserID),
		slog.F("user_name", lbr.UserName),
		slog.F("template_id", lbr.TemplateID),
		slog.F("template_name", lbr.TemplateName),
		slog.F("template_version_id", lbr.TemplateVersionID),
		slog.F("template_version", lbr.TemplateVersion),
		slog.F("workspace_build_id", lbr.WorkspaceBuildID),
		slog.F("workspace_build_transition", lbr.WorkspaceBuildTransition),
		slog.F("job_started_at", lbr.JobStartedAt),
		slog.F("job_completed_at", lbr.JobCompletedAt),
	)

	var events []ResourceUsageEvent
	var added, removed []intermediateTrackedResourceUsage

	log.Debug(ctx, "known resources", slog.F("count", len(r[lbr.WorkspaceID])))
	inters, err := convertWorkspaceBuildInfoToIntermediateTrackedResourceUsage(ctx, log, lbr)
	if err != nil {
		return nil, xerrors.Errorf("convert workspace build info to intermediate tracked resource usage: %w", err)
	}
	log.Debug(ctx, "resources found in state", slog.F("count", len(inters)))

	// Have we seen this workspace before? If not, initialize the map.
	_, alreadySeen := r[lbr.WorkspaceID]

	// If this is the first time we see this workspace, we should assume that all
	// resources are new and being added. We don't do this for a delete
	// transition.
	if !alreadySeen {
		log.Debug(ctx, "initializing workspace in tracker", slog.F("workspace_id", lbr.WorkspaceID))
		r[lbr.WorkspaceID] = make(map[intermediateTrackedResourceUsage]struct{})
		switch lbr.WorkspaceBuildTransition {
		case "stop":
			log.Warn(ctx, "workspace is new to us but transition is stop, we may be missing resources")
		case "delete":
			log.Warn(ctx, "workspace is new to us but transition is delete, not adding resources")
			return []ResourceUsageEvent{}, nil
		case "start":
			log.Debug(ctx, "workspace is new to us, adding all resources")
		default:
			return nil, xerrors.Errorf("unknown workspace build transition: %s", lbr.WorkspaceBuildTransition)
		}
		for _, inter := range inters {
			log.Debug(ctx, "adding all resources", slog.F("resource_id", inter.ResourceID), slog.F("resource_type", inter.ResourceType), slog.F("resource_name", inter.ResourceName))
			added = append(added, inter)
			r[lbr.WorkspaceID][inter] = struct{}{}
		}
		// There will be no events to return for this build.
		return []ResourceUsageEvent{}, nil
	}

	// We have previously seen this workspace!
	if lbr.WorkspaceBuildTransition == "delete" {
		// All resources are removed when the workspace is deleted (theoretically).
		added = []intermediateTrackedResourceUsage{}
		removed = maps.Keys(r[lbr.WorkspaceID])
	} else {
		// Find the set of added and removed resources.
		added, removed = slice.SymmetricDifferenceFunc(maps.Keys(r[lbr.WorkspaceID]), inters, func(a, b intermediateTrackedResourceUsage) bool {
			// Compare the resource ID, type, and name to determine if they are the same.
			return a.ResourceID == b.ResourceID && a.ResourceType == b.ResourceType && a.ResourceName == b.ResourceName
		})
	}
	log.Debug(ctx, "added resources", slog.F("count", len(added)))
	log.Debug(ctx, "removed resources", slog.F("count", len(removed)))

	// Emit an event for each removed resource.
	for _, inter := range removed {
		events = append(events, inter.ToEvent(lbr.JobCompletedAt.UTC()))
	}

	slices.SortFunc(events, func(a, b ResourceUsageEvent) int {
		// Sort by time, then by resource type.
		if cmp := a.Time.Compare(b.Time); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ResourceType, b.ResourceType)
	})

	return events, nil
}

// Terraform recommends against using tfjson directly, and instead defining
// a custom struct for the resources we care about.
type tfstate struct {
	Resources []tfstateResource
}

type tfstateResource struct {
	Mode      string                    `json:"mode"`
	Type      string                    `json:"type"`
	Name      string                    `json:"name"`
	Provider  string                    `json:"provider"`
	Instances []tfstateResourceInstance `json:"instances"`
}

type tfstateResourceInstance struct {
	Attributes map[string]any `json:"attributes"`
}

func (i tfstateResourceInstance) ID() (string, error) {
	// TODO: make this not have to marshal and unmarshal the attributes.
	bs, err := json.Marshal(i.Attributes)
	if err != nil {
		return "", xerrors.Errorf("marshal resource attributes: %w", err)
	}
	tmp := &struct {
		ID string `json:"id"`
	}{}
	if err := json.Unmarshal(bs, &tmp); err != nil {
		return "", err
	}
	return tmp.ID, nil
}

type ResourceUsageEvent struct {
	Time              time.Time         `json:"time"`
	UserID            uuid.UUID         `json:"user_id"`
	UserName          string            `json:"user_name"`
	WorkspaceID       uuid.UUID         `json:"workspace_id"`
	WorkspaceName     string            `json:"workspace_name"`
	TemplateVersionID uuid.UUID         `json:"template_version_id"`
	TemplateVersion   string            `json:"template_version"`
	TemplateID        uuid.UUID         `json:"template_id"`
	TemplateName      string            `json:"template_name"`
	ResourceID        string            `json:"resource_id"`
	ResourceType      string            `json:"resource_type"`
	ResourceName      string            `json:"resource_name"`
	ResourceUnit      string            `json:"unit"`
	ResourceQuantity  decimal.Decimal   `json:"quantity"`
	DurationSeconds   decimal.Decimal   `json:"duration_seconds"`
	Attributes        map[string]string `json:"attributes,omitempty"`
}

func (r ResourceUsageEvent) String() string {
	var sb strings.Builder
	_ = json.NewEncoder(&sb).Encode(r)
	return strings.TrimSpace(sb.String())
}

type WorkspaceBuildInfo struct {
	UserID                   uuid.UUID `db:"user_id"`
	UserName                 string    `db:"user_name"`
	TemplateName             string    `db:"template_name"`
	TemplateID               uuid.UUID `db:"template_id"`
	TemplateVersionID        uuid.UUID `db:"template_version_id"`
	TemplateVersion          string    `db:"template_version"`
	WorkspaceID              uuid.UUID `db:"workspace_id"`
	WorkspaceName            string    `db:"workspace_name"`
	WorkspaceBuildID         uuid.UUID `db:"workspace_build_id"`
	WorkspaceBuildTransition string    `db:"workspace_build_transition"`
	WorkspaceBuildState      []byte    `db:"workspace_build_state"`
	JobStartedAt             time.Time `db:"job_started_at"`
	JobCompletedAt           time.Time `db:"job_completed_at"`
}

type WorkspaceBuildInfoCSVWriter struct {
	w io.Writer
}

func (WorkspaceBuildInfoCSVWriter) header() []string {
	// Returns the CSV header for the workspace build info.
	// This is used for exporting the data to CSV.
	return []string{
		"user_id",
		"user_name",
		"template_name",
		"template_id",
		"template_version_id",
		"template_version",
		"workspace_id",
		"workspace_name",
		"workspace_build_id",
		"workspace_build_transition",
		"workspace_build_state",
		"job_started_at",
		"job_completed_at",
	}
}

func (w WorkspaceBuildInfoCSVWriter) Write(entries ...WorkspaceBuildInfo) error {
	// Returns a CSV representation of the workspace build info.
	// This is used for exporting the data to CSV.
	csvWriter := csv.NewWriter(w.w)
	if err := csvWriter.Write(w.header()); err != nil {
		return xerrors.Errorf("write CSV header: %w", err)
	}
	var sb strings.Builder
	for idx, entry := range entries {
		stateEnc := base64.NewEncoder(base64.StdEncoding, &sb)
		// Encode the workspace build state as base64 to avoid issues with special
		// characters.
		if _, err := stateEnc.Write(entry.WorkspaceBuildState); err != nil {
			return xerrors.Errorf("encode workspace build state for entry %d: %w", idx, err)
		}
		if err := stateEnc.Close(); err != nil {
			return xerrors.Errorf("close base64 encoder for entry %d: %w", idx, err)
		}
		encState := sb.String()
		if err := csvWriter.Write([]string{
			entry.UserID.String(),
			entry.UserName,
			entry.TemplateName,
			entry.TemplateID.String(),
			entry.TemplateVersionID.String(),
			entry.TemplateVersion,
			entry.WorkspaceID.String(),
			entry.WorkspaceName,
			entry.WorkspaceBuildID.String(),
			entry.WorkspaceBuildTransition,
			encState,
			entry.JobStartedAt.Format(time.RFC3339Nano),
			entry.JobCompletedAt.Format(time.RFC3339Nano),
		}); err != nil {
			return xerrors.Errorf("write CSV entry %d: %w", idx, err)
		}
		sb.Reset()
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return xerrors.Errorf("flush CSV writer: %w", err)
	}
	return nil
}

type WorkspaceBuildInfoCSVReader struct {
	R   io.Reader
	log slog.Logger
}

func (r WorkspaceBuildInfoCSVReader) Read() ([]WorkspaceBuildInfo, error) {
	// Reads a CSV representation of the workspace build info.
	// This is used for importing the data from CSV.
	csvReader := csv.NewReader(r.R)
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, xerrors.Errorf("read CSV: %w", err)
	}

	r.log.Debug(context.Background(), "read workspace builds from CSV",
		slog.F("count", len(records)))

	var builds []WorkspaceBuildInfo
	for idx, record := range records {
		if idx == 0 {
			// Skip the header row.
			continue
		}
		build, err := r.handleRecord(record)
		if err != nil {
			r.log.Error(context.Background(), "handle record", slog.Error(err), slog.F("idx", idx))
			continue
		}
		builds = append(builds, build)
	}
	return builds, nil
}

func decodeWorkspaceBuildState(s string) ([]byte, error) {
	// Try base64 first
	b64, err := base64.StdEncoding.DecodeString(s)
	if err == nil {
		if errors.Is(err, io.EOF) {
			return []byte{}, nil
		}
		return b64, nil
	}
	// Try hex (Postgres bytea hex format: \x...)
	if strings.HasPrefix(s, "\\x") {
		return hex.DecodeString(strings.TrimPrefix(s, "\\x"))
	}
	return nil, xerrors.Errorf("could not decode workspace build state as base64 or hex: %w", err)
}

var postgresTimestampFormat = "2006-01-02 15:04:05.999999999-07"

func parseTimestamp(s string) (time.Time, error) {
	// First try to parse as RFC3339Nano
	t, err := time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return t, nil
	}

	// If that fails, try PostgreSQL's default timestamp format
	t, err = time.Parse(postgresTimestampFormat, s)
	if err == nil {
		return t, nil
	}

	return time.Time{}, xerrors.Errorf("could not parse time: %w", err)
}

func (WorkspaceBuildInfoCSVReader) handleRecord(record []string) (WorkspaceBuildInfo, error) {
	if len(record) != 13 {
		return WorkspaceBuildInfo{}, xerrors.Errorf("invalid record length: %d", len(record))
	}

	userID, err := uuid.Parse(record[0])
	if err != nil {
		return WorkspaceBuildInfo{}, xerrors.Errorf("parse user ID: %w", err)
	}
	templateID, err := uuid.Parse(record[3])
	if err != nil {
		return WorkspaceBuildInfo{}, xerrors.Errorf("parse template ID: %w", err)
	}
	templateVersionID, err := uuid.Parse(record[4])
	if err != nil {
		return WorkspaceBuildInfo{}, xerrors.Errorf("parse template version ID: %w", err)
	}
	workspaceID, err := uuid.Parse(record[6])
	if err != nil {
		return WorkspaceBuildInfo{}, xerrors.Errorf("parse workspace ID: %w", err)
	}
	workspaceBuildID, err := uuid.Parse(record[8])
	if err != nil {
		return WorkspaceBuildInfo{}, xerrors.Errorf("parse workspace build ID: %w", err)
	}
	// Decode the workspace build state (base64 or hex fallback)
	decState, err := decodeWorkspaceBuildState(record[10])
	if err != nil {
		return WorkspaceBuildInfo{}, xerrors.Errorf("decode workspace build state: %w", err)
	}
	jobStartedAt, err := parseTimestamp(record[11])
	if err != nil {
		return WorkspaceBuildInfo{}, xerrors.Errorf("parse job started at: %w", err)
	}
	jobCompletedAt, err := parseTimestamp(record[12])
	if err != nil {
		return WorkspaceBuildInfo{}, xerrors.Errorf("parse job completed at: %w", err)
	}

	return WorkspaceBuildInfo{
		UserID:                   userID,
		UserName:                 record[1],
		TemplateName:             record[2],
		TemplateID:               templateID,
		TemplateVersionID:        templateVersionID,
		TemplateVersion:          record[5],
		WorkspaceID:              workspaceID,
		WorkspaceName:            record[7],
		WorkspaceBuildID:         workspaceBuildID,
		WorkspaceBuildTransition: record[9],
		WorkspaceBuildState:      decState,
		JobStartedAt:             jobStartedAt,
		JobCompletedAt:           jobCompletedAt,
	}, nil
}

const queryListBuilds = `
SELECT
	u.id AS user_id,
	u.username AS user_name,
	t.name AS template_name,
	t.id AS template_id,
	tv.id AS template_version_id,
	tv.name AS template_version,
	w.id AS workspace_id,
	w.name AS workspace_name,
	wb.id AS workspace_build_id,
	wb.transition AS workspace_build_transition,
	wb.provisioner_state AS workspace_build_state,
	pj.started_at AS job_started_at,
	pj.completed_at AS job_completed_at
  FROM workspace_builds wb
	JOIN workspaces w ON wb.workspace_id = w.id
	JOIN users u ON w.owner_id = u.id
	JOIN template_versions tv ON wb.template_version_id = tv.id
	JOIN templates t ON tv.template_id = t.id
	JOIN provisioner_jobs pj ON wb.job_id = pj.id
WHERE
	-- Only include jobs that have started.
	pj.started_at IS NOT NULL
AND
	-- Only include jobs that have completed.
	pj.completed_at IS NOT NULL
AND
  -- Only include jobs that have completed successfully.
  pj.error IS NULL
AND
  CASE WHEN $1::timestamptz IS NOT NULL THEN
		pj.created_at >= $1::timestamptz
	ELSE TRUE END
AND
	CASE WHEN $2::timestamptz IS NOT NULL THEN
		pj.completed_at IS NOT NULL AND pj.completed_at <= $2::timestamptz
	ELSE TRUE END
ORDER BY
	pj.completed_at ASC
;`

func listBuilds(ctx context.Context, logger slog.Logger, sqlDB *sql.DB, fromTime, toTime codersdk.NullTime) ([]WorkspaceBuildInfo, error) {
	rows, err := sqlDB.QueryContext(ctx, queryListBuilds, fromTime, toTime)
	if err != nil {
		return nil, xerrors.Errorf("query workspace builds: %w", err)
	}
	defer rows.Close()

	var builds []WorkspaceBuildInfo
	for rows.Next() {
		var build WorkspaceBuildInfo
		if err := rows.Scan(
			&build.UserID,
			&build.UserName,
			&build.TemplateName,
			&build.TemplateID,
			&build.TemplateVersionID,
			&build.TemplateVersion,
			&build.WorkspaceID,
			&build.WorkspaceName,
			&build.WorkspaceBuildID,
			&build.WorkspaceBuildTransition,
			&build.WorkspaceBuildState,
			&build.JobStartedAt,
			&build.JobCompletedAt,
		); err != nil {
			return nil, xerrors.Errorf("scan workspace build: %w", err)
		}
		builds = append(builds, build)
	}
	if err := rows.Err(); err != nil {
		return nil, xerrors.Errorf("iterate workspace builds: %w", err)
	}
	return builds, nil
}

const queryInsertEvents = `INSERT INTO events (event_type, created_at, data) VALUES ($1, $2, $3);`

func insertEvents(ctx context.Context, logger slog.Logger, sqlDB *sql.DB, events []ResourceUsageEvent) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	for _, event := range events {
		if _, err := tx.ExecContext(ctx, queryInsertEvents,
			"resource_usage",
			event.Time.UTC(),
			[]byte(event.String()),
		); err != nil {
			return xerrors.Errorf("insert event: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("commit transaction: %w", err)
	}
	logger.Debug(ctx, "inserted resource usage events", slog.F("count", len(events)))
	return nil
}

type eventWriter func(ctx context.Context, event ...ResourceUsageEvent) error

func stdoutEventWriter(w io.Writer) eventWriter {
	return func(_ context.Context, events ...ResourceUsageEvent) error {
		for _, evt := range events {
			if _, err := fmt.Fprintf(w, "%s\n", evt.String()); err != nil {
				return err
			}
		}
		return nil
	}
}

func sqlEventWriter(logger slog.Logger, sqlDB *sql.DB) eventWriter {
	return func(ctx context.Context, events ...ResourceUsageEvent) error {
		if err := insertEvents(ctx, logger, sqlDB, events); err != nil {
			return xerrors.Errorf("insert event: %w", err)
		}
		return nil
	}
}

// resourceUsageQuantity represents usage of a resource in a specific unit and quantity.
type resourceUsageQuantity struct {
	// Unit is the unit of measurement for the resource usage, e.g., "cpu", "memory", etc.
	Unit string `json:"unit"`
	// Quantity is the amount of the resource used, in the specified unit.
	Quantity decimal.Decimal `json:"quantity"`
	// Attributes are any additional attributes that may influence the final cost
	// of the resource usage.
	Attributes map[string]string `json:"attributes"`
}

// resourceUsageExtractor is used to extract relevant usage information from a
// terraform state resource.
type resourceUsageExtractor struct {
	// Unit is the unit of measurement for the resource usage, e.g., "cores",
	// "megabytes", etc. Always prefer to use SI units.
	Unit string
	// ValuePath is a JSONPath expression that returns the value of the resource usage.
	ValuePath string
	// AttributePaths is a map of attribute names to JSONPath expression that should be used
	// to extract additional attributes that may influence the final cost of the resource usage.
	AttributePaths map[string]string
	// Convert is a function that converts the raw value extracted from the
	// resource attributes to a decimal.Decimal.
	Convert func(raw interface{}) (decimal.Decimal, error)
}

func (r resourceUsageExtractor) Extract(resInst tfstateResourceInstance) (resourceUsageQuantity, error) {
	resInstID, err := resInst.ID()
	if err != nil {
		return resourceUsageQuantity{}, xerrors.Errorf("get resource instance ID: %w", err)
	}
	us := resourceUsageQuantity{
		Attributes: make(map[string]string, len(r.AttributePaths)),
		Unit:       r.Unit,
		Quantity:   decimal.Zero,
	}
	if r.ValuePath == "" {
		return resourceUsageQuantity{}, xerrors.New("value path must not be empty")
	}
	rawValue, err := jsonpath.Get(r.ValuePath, resInst.Attributes)
	if err != nil {
		return resourceUsageQuantity{}, xerrors.Errorf("extract value from resource attributes: %w", err)
	}
	if rawValue == nil {
		return resourceUsageQuantity{}, xerrors.Errorf("value path %q returned nil for resource %s", r.ValuePath, resInstID)
	}
	if r.Unit == "" {
		return resourceUsageQuantity{}, xerrors.New("unit must not be empty")
	}

	convert := r.Convert
	if convert == nil {
		convert = convertDefault
	}
	v, err := convert(rawValue)
	if err != nil {
		return resourceUsageQuantity{}, xerrors.Errorf("convert value at path %q for resource %s: %w", r.ValuePath, resInstID, err)
	}
	us.Quantity = v

	for attrName, attrPath := range r.AttributePaths {
		rawAttrValue, err := jsonpath.Get(attrPath, resInst.Attributes)
		if err != nil {
			return resourceUsageQuantity{}, xerrors.Errorf("extract attribute %q from resource attributes: %w", attrName, err)
		}
		if rawAttrValue == nil {
			return resourceUsageQuantity{}, xerrors.Errorf("attribute path %q returned nil for resource %s", attrPath, resInstID)
		}
		switch v := rawAttrValue.(type) {
		case string:
			us.Attributes[attrName] = v
		default:
			return resourceUsageQuantity{}, xerrors.Errorf("unexpected attribute type %T for attribute path %q in resource %s", v, attrPath, resInstID)
		}
	}

	return us, nil
}

func convertDefault(raw interface{}) (decimal.Decimal, error) {
	if raw == nil {
		return decimal.Zero, xerrors.New("raw value is nil")
	}
	switch v := raw.(type) {
	case string:
		return decimal.NewFromString(v)
	case int64:
		return decimal.NewFromInt(v), nil
	case int:
		return decimal.NewFromInt(int64(v)), nil
	case float64:
		return decimal.NewFromFloat(v), nil
	default:
		return decimal.Zero, xerrors.Errorf("unexpected value type %T for conversion to decimal", v)
	}
}

func ConvertSIString(raw interface{}) (decimal.Decimal, error) {
	if raw == nil {
		return decimal.Zero, xerrors.New("raw value is nil")
	}
	switch v := raw.(type) {
	case string:
		q, err := kresource.ParseQuantity(v)
		if err != nil {
			return decimal.Zero, err
		}
		// Convert the quantity to a decimal.Decimal.
		if q.IsZero() {
			return decimal.Zero, nil
		}
		return decimal.NewFromFloat(q.AsFloat64Slow()), nil
	default:
		return decimal.Zero, xerrors.Errorf("unexpected value type %T for SI string conversion", v)
	}
}

var defaultResourceUsageExtractors = map[string][]resourceUsageExtractor{
	"kubernetes_persistent_volume_claim": {
		{
			Unit:      "disk_bytes",
			ValuePath: "$.spec[0].resources[0].requests.storage",
			AttributePaths: map[string]string{
				"namespace":     "$.metadata[0].namespace",
				"storage_class": "$.spec[0].storage_class_name",
			},
			Convert: ConvertSIString,
		},
	},
	"kubernetes_deployment": {
		{
			Unit:      "cpu_cores",
			ValuePath: "$.spec[0].template[0].spec[0].container[0].resources[0].requests.cpu",
			AttributePaths: map[string]string{
				"namespace": "$.metadata[0].namespace",
			},
			Convert: ConvertSIString,
		},
		{
			Unit:      "memory_bytes",
			ValuePath: "$.spec[0].template[0].spec[0].container[0].resources[0].requests.memory",
			AttributePaths: map[string]string{
				"namespace": "$.metadata[0].namespace",
			},
			Convert: ConvertSIString,
		},
	},
	"aws_instance": {
		{
			Unit:      "disk_bytes",
			ValuePath: "$.root_block_device[0].volume_size",
			AttributePaths: map[string]string{
				"availability_zone": "$.availability_zone",
				"instance_type":     "$.instance_type",
				"volume_type":       "$.root_block_device[0].volume_type",
			},
			Convert: convertDefault,
		},
	},
}
