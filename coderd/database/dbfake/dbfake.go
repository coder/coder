package dbfake

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/telemetry"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

// Workspace inserts a workspace into the database.
func Workspace(t testing.TB, db database.Store, seed database.Workspace) database.Workspace {
	t.Helper()

	// This intentionally fulfills the minimum requirements of the schema.
	// Tests can provide a custom template ID if necessary.
	if seed.TemplateID == uuid.Nil {
		template := dbgen.Template(t, db, database.Template{
			OrganizationID: seed.OrganizationID,
			CreatedBy:      seed.OwnerID,
		})
		seed.TemplateID = template.ID
		seed.OwnerID = template.CreatedBy
		seed.OrganizationID = template.OrganizationID
	}
	return dbgen.Workspace(t, db, seed)
}

// WorkspaceWithAgent is a helper that generates a workspace with a single resource
// that has an agent attached to it. The agent token is returned.
func WorkspaceWithAgent(t testing.TB, db database.Store, seed database.Workspace) (database.Workspace, string) {
	t.Helper()
	authToken := uuid.NewString()
	ws := Workspace(t, db, seed)
	WorkspaceBuild(t, db, ws, database.WorkspaceBuild{}, &sdkproto.Resource{
		Name: "example",
		Type: "aws_instance",
		Agents: []*sdkproto.Agent{{
			Id: uuid.NewString(),
			Auth: &sdkproto.Agent_Token{
				Token: authToken,
			},
		}},
	})
	return ws, authToken
}

// WorkspaceBuild inserts a build and a successful job into the database.
func WorkspaceBuild(t testing.TB, db database.Store, ws database.Workspace, seed database.WorkspaceBuild, resources ...*sdkproto.Resource) database.WorkspaceBuild {
	t.Helper()
	jobID := uuid.New()
	seed.ID = uuid.New()
	seed.JobID = jobID
	seed.WorkspaceID = ws.ID

	// Create a provisioner job for the build!
	payload, err := json.Marshal(provisionerdserver.WorkspaceProvisionJob{
		WorkspaceBuildID: seed.ID,
	})
	require.NoError(t, err)
	//nolint:gocritic // This is only used by tests.
	ctx := dbauthz.AsSystemRestricted(context.Background())
	job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
		ID:             jobID,
		CreatedAt:      dbtime.Now(),
		UpdatedAt:      dbtime.Now(),
		OrganizationID: ws.OrganizationID,
		InitiatorID:    ws.OwnerID,
		Provisioner:    database.ProvisionerTypeEcho,
		StorageMethod:  database.ProvisionerStorageMethodFile,
		FileID:         uuid.New(),
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		Input:          payload,
		Tags:           nil,
		TraceMetadata:  pqtype.NullRawMessage{},
	})
	require.NoError(t, err, "insert job")
	err = db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
		ID:        job.ID,
		UpdatedAt: dbtime.Now(),
		Error:     sql.NullString{},
		ErrorCode: sql.NullString{},
		CompletedAt: sql.NullTime{
			Time:  dbtime.Now(),
			Valid: true,
		},
	})
	require.NoError(t, err, "complete job")

		rows = append(rows, database.GetTemplateAppInsightsRow{
			TemplateIDs:   templateIDs,
			ActiveUserIDs: activeUserIDs,
			AccessMethod:  appKey.AccessMethod,
			SlugOrPort:    appKey.SlugOrPort,
			DisplayName:   sql.NullString{String: appKey.DisplayName, Valid: appKey.DisplayName != ""},
			Icon:          sql.NullString{String: appKey.Icon, Valid: appKey.Icon != ""},
			IsApp:         appKey.Slug != "",
			UsageSeconds:  usage,
		})
	}

	// NOTE(mafredri): Add sorting if we decide on how to handle PostgreSQL collations.
	// ORDER BY access_method, slug_or_port, display_name, icon, is_app
	return rows, nil
}

func (q *FakeQuerier) GetTemplateAverageBuildTime(ctx context.Context, arg database.GetTemplateAverageBuildTimeParams) (database.GetTemplateAverageBuildTimeRow, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.GetTemplateAverageBuildTimeRow{}, err
	}

	var emptyRow database.GetTemplateAverageBuildTimeRow
	var (
		startTimes  []float64
		stopTimes   []float64
		deleteTimes []float64
	)
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	for _, wb := range q.workspaceBuilds {
		version, err := q.getTemplateVersionByIDNoLock(ctx, wb.TemplateVersionID)
		if err != nil {
			return emptyRow, err
		}
		if version.TemplateID != arg.TemplateID {
			continue
		}

		job, err := q.getProvisionerJobByIDNoLock(ctx, wb.JobID)
		if err != nil {
			return emptyRow, err
		}
		if job.CompletedAt.Valid {
			took := job.CompletedAt.Time.Sub(job.StartedAt.Time).Seconds()
			switch wb.Transition {
			case database.WorkspaceTransitionStart:
				startTimes = append(startTimes, took)
			case database.WorkspaceTransitionStop:
				stopTimes = append(stopTimes, took)
			case database.WorkspaceTransitionDelete:
				deleteTimes = append(deleteTimes, took)
			}
		}
	}

	tryPercentile := func(fs []float64, p float64) float64 {
		if len(fs) == 0 {
			return -1
		}
		sort.Float64s(fs)
		return fs[int(float64(len(fs))*p/100)]
	}

	var row database.GetTemplateAverageBuildTimeRow
	row.Delete50, row.Delete95 = tryPercentile(deleteTimes, 50), tryPercentile(deleteTimes, 95)
	row.Stop50, row.Stop95 = tryPercentile(stopTimes, 50), tryPercentile(stopTimes, 95)
	row.Start50, row.Start95 = tryPercentile(startTimes, 50), tryPercentile(startTimes, 95)
	return row, nil
}

func (q *FakeQuerier) GetTemplateByID(ctx context.Context, id uuid.UUID) (database.Template, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getTemplateByIDNoLock(ctx, id)
}

func (q *FakeQuerier) GetTemplateByOrganizationAndName(_ context.Context, arg database.GetTemplateByOrganizationAndNameParams) (database.Template, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Template{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, template := range q.templates {
		if template.OrganizationID != arg.OrganizationID {
			continue
		}
		if !strings.EqualFold(template.Name, arg.Name) {
			continue
		}
		if template.Deleted != arg.Deleted {
			continue
		}
		return q.templateWithUserNoLock(template), nil
	}
	return database.Template{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetTemplateDAUs(_ context.Context, arg database.GetTemplateDAUsParams) ([]database.GetTemplateDAUsRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	seens := make(map[time.Time]map[uuid.UUID]struct{})

	for _, as := range q.workspaceAgentStats {
		if as.TemplateID != arg.TemplateID {
			continue
		}
		if as.ConnectionCount == 0 {
			continue
		}

		date := as.CreatedAt.UTC().Add(time.Duration(arg.TzOffset) * time.Hour * -1).Truncate(time.Hour * 24)

		dateEntry := seens[date]
		if dateEntry == nil {
			dateEntry = make(map[uuid.UUID]struct{})
		}
		dateEntry[as.UserID] = struct{}{}
		seens[date] = dateEntry
	}

	seenKeys := maps.Keys(seens)
	sort.Slice(seenKeys, func(i, j int) bool {
		return seenKeys[i].Before(seenKeys[j])
	})

	var rs []database.GetTemplateDAUsRow
	for _, key := range seenKeys {
		ids := seens[key]
		for id := range ids {
			rs = append(rs, database.GetTemplateDAUsRow{
				Date:   key,
				UserID: id,
			})
		}
	}

	return rs, nil
}

func (q *FakeQuerier) GetTemplateInsights(_ context.Context, arg database.GetTemplateInsightsParams) (database.GetTemplateInsightsRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.GetTemplateInsightsRow{}, err
	}

	templateIDSet := make(map[uuid.UUID]struct{})
	appUsageIntervalsByUser := make(map[uuid.UUID]map[time.Time]*database.GetTemplateInsightsRow)

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, s := range q.workspaceAgentStats {
		if s.CreatedAt.Before(arg.StartTime) || s.CreatedAt.Equal(arg.EndTime) || s.CreatedAt.After(arg.EndTime) {
			continue
		}
		if len(arg.TemplateIDs) > 0 && !slices.Contains(arg.TemplateIDs, s.TemplateID) {
			continue
		}
		if s.ConnectionCount == 0 {
			continue
		}

		templateIDSet[s.TemplateID] = struct{}{}
		if appUsageIntervalsByUser[s.UserID] == nil {
			appUsageIntervalsByUser[s.UserID] = make(map[time.Time]*database.GetTemplateInsightsRow)
		}
		t := s.CreatedAt.Truncate(time.Minute)
		if _, ok := appUsageIntervalsByUser[s.UserID][t]; !ok {
			appUsageIntervalsByUser[s.UserID][t] = &database.GetTemplateInsightsRow{}
		}

		if s.SessionCountJetBrains > 0 {
			appUsageIntervalsByUser[s.UserID][t].UsageJetbrainsSeconds = 60
		}
		if s.SessionCountVSCode > 0 {
			appUsageIntervalsByUser[s.UserID][t].UsageVscodeSeconds = 60
		}
		if s.SessionCountReconnectingPTY > 0 {
			appUsageIntervalsByUser[s.UserID][t].UsageReconnectingPtySeconds = 60
		}
		if s.SessionCountSSH > 0 {
			appUsageIntervalsByUser[s.UserID][t].UsageSshSeconds = 60
		}
	}

	templateIDs := make([]uuid.UUID, 0, len(templateIDSet))
	for templateID := range templateIDSet {
		templateIDs = append(templateIDs, templateID)
	}
	slices.SortFunc(templateIDs, func(a, b uuid.UUID) int {
		return slice.Ascending(a.String(), b.String())
	})
	activeUserIDs := make([]uuid.UUID, 0, len(appUsageIntervalsByUser))
	for userID := range appUsageIntervalsByUser {
		activeUserIDs = append(activeUserIDs, userID)
	}

	result := database.GetTemplateInsightsRow{
		TemplateIDs:   templateIDs,
		ActiveUserIDs: activeUserIDs,
	}
	for _, intervals := range appUsageIntervalsByUser {
		for _, interval := range intervals {
			result.UsageJetbrainsSeconds += interval.UsageJetbrainsSeconds
			result.UsageVscodeSeconds += interval.UsageVscodeSeconds
			result.UsageReconnectingPtySeconds += interval.UsageReconnectingPtySeconds
			result.UsageSshSeconds += interval.UsageSshSeconds
		}
	}
	return result, nil
}

func (q *FakeQuerier) GetTemplateInsightsByInterval(ctx context.Context, arg database.GetTemplateInsightsByIntervalParams) ([]database.GetTemplateInsightsByIntervalRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	type statByInterval struct {
		startTime, endTime time.Time
		userSet            map[uuid.UUID]struct{}
		templateIDSet      map[uuid.UUID]struct{}
	}

	statsByInterval := []statByInterval{{arg.StartTime, arg.StartTime.AddDate(0, 0, int(arg.IntervalDays)), make(map[uuid.UUID]struct{}), make(map[uuid.UUID]struct{})}}
	for statsByInterval[len(statsByInterval)-1].endTime.Before(arg.EndTime) {
		statsByInterval = append(statsByInterval, statByInterval{statsByInterval[len(statsByInterval)-1].endTime, statsByInterval[len(statsByInterval)-1].endTime.AddDate(0, 0, int(arg.IntervalDays)), make(map[uuid.UUID]struct{}), make(map[uuid.UUID]struct{})})
	}
	if statsByInterval[len(statsByInterval)-1].endTime.After(arg.EndTime) {
		statsByInterval[len(statsByInterval)-1].endTime = arg.EndTime
	}

	for _, s := range q.workspaceAgentStats {
		if s.CreatedAt.Before(arg.StartTime) || s.CreatedAt.Equal(arg.EndTime) || s.CreatedAt.After(arg.EndTime) {
			continue
		}
		if len(arg.TemplateIDs) > 0 && !slices.Contains(arg.TemplateIDs, s.TemplateID) {
			continue
		}
		if s.ConnectionCount == 0 {
			continue
		}

		for _, ds := range statsByInterval {
			if s.CreatedAt.Before(ds.startTime) || s.CreatedAt.Equal(ds.endTime) || s.CreatedAt.After(ds.endTime) {
				continue
			}
			ds.userSet[s.UserID] = struct{}{}
			ds.templateIDSet[s.TemplateID] = struct{}{}
		}
	}

	for _, s := range q.workspaceAppStats {
		w, err := q.getWorkspaceByIDNoLock(ctx, s.WorkspaceID)
		if err != nil {
			return nil, err
		}

		if len(arg.TemplateIDs) > 0 && !slices.Contains(arg.TemplateIDs, w.TemplateID) {
			continue
		}

		for _, ds := range statsByInterval {
			// (was.session_started_at >= ts.from_ AND was.session_started_at < ts.to_)
			// OR (was.session_ended_at > ts.from_ AND was.session_ended_at < ts.to_)
			// OR (was.session_started_at < ts.from_ AND was.session_ended_at >= ts.to_)
			if !(((s.SessionStartedAt.After(ds.startTime) || s.SessionStartedAt.Equal(ds.startTime)) && s.SessionStartedAt.Before(ds.endTime)) ||
				(s.SessionEndedAt.After(ds.startTime) && s.SessionEndedAt.Before(ds.endTime)) ||
				(s.SessionStartedAt.Before(ds.startTime) && (s.SessionEndedAt.After(ds.endTime) || s.SessionEndedAt.Equal(ds.endTime)))) {
				continue
			}

			ds.userSet[s.UserID] = struct{}{}
			ds.templateIDSet[w.TemplateID] = struct{}{}
		}
	}

	var result []database.GetTemplateInsightsByIntervalRow
	for _, ds := range statsByInterval {
		templateIDs := make([]uuid.UUID, 0, len(ds.templateIDSet))
		for templateID := range ds.templateIDSet {
			templateIDs = append(templateIDs, templateID)
		}
		slices.SortFunc(templateIDs, func(a, b uuid.UUID) int {
			return slice.Ascending(a.String(), b.String())
		})
		result = append(result, database.GetTemplateInsightsByIntervalRow{
			StartTime:   ds.startTime,
			EndTime:     ds.endTime,
			TemplateIDs: templateIDs,
			ActiveUsers: int64(len(ds.userSet)),
		})
	}
	return result, nil
}

func (q *FakeQuerier) GetTemplateInsightsByTemplate(_ context.Context, arg database.GetTemplateInsightsByTemplateParams) ([]database.GetTemplateInsightsByTemplateRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// map time.Time x TemplateID x UserID x <usage>
	appUsageByTemplateAndUser := map[time.Time]map[uuid.UUID]map[uuid.UUID]database.GetTemplateInsightsByTemplateRow{}

	// Review agent stats in terms of usage
	templateIDSet := make(map[uuid.UUID]struct{})

	for _, s := range q.workspaceAgentStats {
		if s.CreatedAt.Before(arg.StartTime) || s.CreatedAt.Equal(arg.EndTime) || s.CreatedAt.After(arg.EndTime) {
			continue
		}
		if s.ConnectionCount == 0 {
			continue
		}

		t := s.CreatedAt.Truncate(time.Minute)
		templateIDSet[s.TemplateID] = struct{}{}

		if _, ok := appUsageByTemplateAndUser[t]; !ok {
			appUsageByTemplateAndUser[t] = make(map[uuid.UUID]map[uuid.UUID]database.GetTemplateInsightsByTemplateRow)
		}

		if _, ok := appUsageByTemplateAndUser[t][s.TemplateID]; !ok {
			appUsageByTemplateAndUser[t][s.TemplateID] = make(map[uuid.UUID]database.GetTemplateInsightsByTemplateRow)
		}

		if _, ok := appUsageByTemplateAndUser[t][s.TemplateID][s.UserID]; !ok {
			appUsageByTemplateAndUser[t][s.TemplateID][s.UserID] = database.GetTemplateInsightsByTemplateRow{}
		}

		u := appUsageByTemplateAndUser[t][s.TemplateID][s.UserID]
		if s.SessionCountJetBrains > 0 {
			u.UsageJetbrainsSeconds = 60
		}
		if s.SessionCountVSCode > 0 {
			u.UsageVscodeSeconds = 60
		}
		if s.SessionCountReconnectingPTY > 0 {
			u.UsageReconnectingPtySeconds = 60
		}
		if s.SessionCountSSH > 0 {
			u.UsageSshSeconds = 60
		}
		appUsageByTemplateAndUser[t][s.TemplateID][s.UserID] = u
	}

	// Sort used templates
	templateIDs := make([]uuid.UUID, 0, len(templateIDSet))
	for templateID := range templateIDSet {
		templateIDs = append(templateIDs, templateID)
	}
	slices.SortFunc(templateIDs, func(a, b uuid.UUID) int {
		return slice.Ascending(a.String(), b.String())
	})

	// Build result
	var result []database.GetTemplateInsightsByTemplateRow
	for _, templateID := range templateIDs {
		r := database.GetTemplateInsightsByTemplateRow{
			TemplateID: templateID,
		}

		uniqueUsers := map[uuid.UUID]struct{}{}

		for _, mTemplateUserUsage := range appUsageByTemplateAndUser {
			mUserUsage, ok := mTemplateUserUsage[templateID]
			if !ok {
				continue // template was not used in this time window
			}

			for userID, usage := range mUserUsage {
				uniqueUsers[userID] = struct{}{}

				r.UsageJetbrainsSeconds += usage.UsageJetbrainsSeconds
				r.UsageVscodeSeconds += usage.UsageVscodeSeconds
				r.UsageReconnectingPtySeconds += usage.UsageReconnectingPtySeconds
				r.UsageSshSeconds += usage.UsageSshSeconds
			}
		}

		r.ActiveUsers = int64(len(uniqueUsers))

		result = append(result, r)
	}
	return result, nil
}

func (q *FakeQuerier) GetTemplateParameterInsights(ctx context.Context, arg database.GetTemplateParameterInsightsParams) ([]database.GetTemplateParameterInsightsRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// WITH latest_workspace_builds ...
	latestWorkspaceBuilds := make(map[uuid.UUID]database.WorkspaceBuildTable)
	for _, wb := range q.workspaceBuilds {
		if wb.CreatedAt.Before(arg.StartTime) || wb.CreatedAt.Equal(arg.EndTime) || wb.CreatedAt.After(arg.EndTime) {
			continue
		}
		if latestWorkspaceBuilds[wb.WorkspaceID].BuildNumber < wb.BuildNumber {
			latestWorkspaceBuilds[wb.WorkspaceID] = wb
		}
	}
	if len(arg.TemplateIDs) > 0 {
		for wsID := range latestWorkspaceBuilds {
			ws, err := q.getWorkspaceByIDNoLock(ctx, wsID)
			if err != nil {
				return nil, err
			}
			if slices.Contains(arg.TemplateIDs, ws.TemplateID) {
				delete(latestWorkspaceBuilds, wsID)
			}
		}
	}
	// WITH unique_template_params ...
	num := int64(0)
	uniqueTemplateParams := make(map[string]*database.GetTemplateParameterInsightsRow)
	uniqueTemplateParamWorkspaceBuildIDs := make(map[string][]uuid.UUID)
	for _, wb := range latestWorkspaceBuilds {
		tv, err := q.getTemplateVersionByIDNoLock(ctx, wb.TemplateVersionID)
		if err != nil {
			return nil, err
		}
		for _, tvp := range q.templateVersionParameters {
			if tvp.TemplateVersionID != tv.ID {
				continue
			}
			// GROUP BY tvp.name, tvp.type, tvp.display_name, tvp.description, tvp.options
			key := fmt.Sprintf("%s:%s:%s:%s:%s", tvp.Name, tvp.Type, tvp.DisplayName, tvp.Description, tvp.Options)
			if _, ok := uniqueTemplateParams[key]; !ok {
				num++
				uniqueTemplateParams[key] = &database.GetTemplateParameterInsightsRow{
					Num:         num,
					Name:        tvp.Name,
					Type:        tvp.Type,
					DisplayName: tvp.DisplayName,
					Description: tvp.Description,
					Options:     tvp.Options,
				}
			}
			uniqueTemplateParams[key].TemplateIDs = append(uniqueTemplateParams[key].TemplateIDs, tv.TemplateID.UUID)
			uniqueTemplateParamWorkspaceBuildIDs[key] = append(uniqueTemplateParamWorkspaceBuildIDs[key], wb.ID)
		}
	}
	// SELECT ...
	counts := make(map[string]map[string]int64)
	for key, utp := range uniqueTemplateParams {
		for _, wbp := range q.workspaceBuildParameters {
			if !slices.Contains(uniqueTemplateParamWorkspaceBuildIDs[key], wbp.WorkspaceBuildID) {
				continue
			}
			if wbp.Name != utp.Name {
				continue
			}
			if counts[key] == nil {
				counts[key] = make(map[string]int64)
			}
			counts[key][wbp.Value]++
		}
	}

	var rows []database.GetTemplateParameterInsightsRow
	for key, utp := range uniqueTemplateParams {
		for value, count := range counts[key] {
			rows = append(rows, database.GetTemplateParameterInsightsRow{
				Num:         utp.Num,
				TemplateIDs: uniqueSortedUUIDs(utp.TemplateIDs),
				Name:        utp.Name,
				DisplayName: utp.DisplayName,
				Type:        utp.Type,
				Description: utp.Description,
				Options:     utp.Options,
				Value:       value,
				Count:       count,
			})
		}
	}

	// NOTE(mafredri): Add sorting if we decide on how to handle PostgreSQL collations.
	// ORDER BY utp.name, utp.type, utp.display_name, utp.description, utp.options, wbp.value
	return rows, nil
}

func (q *FakeQuerier) GetTemplateVersionByID(ctx context.Context, templateVersionID uuid.UUID) (database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getTemplateVersionByIDNoLock(ctx, templateVersionID)
}

func (q *FakeQuerier) GetTemplateVersionByJobID(_ context.Context, jobID uuid.UUID) (database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, templateVersion := range q.templateVersions {
		if templateVersion.JobID != jobID {
			continue
		}
		return q.templateVersionWithUserNoLock(templateVersion), nil
	}
	return database.TemplateVersion{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetTemplateVersionByTemplateIDAndName(_ context.Context, arg database.GetTemplateVersionByTemplateIDAndNameParams) (database.TemplateVersion, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.TemplateVersion{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, templateVersion := range q.templateVersions {
		if templateVersion.TemplateID != arg.TemplateID {
			continue
		}
		if !strings.EqualFold(templateVersion.Name, arg.Name) {
			continue
		}
		return q.templateVersionWithUserNoLock(templateVersion), nil
	}
	return database.TemplateVersion{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetTemplateVersionParameters(_ context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionParameter, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	parameters := make([]database.TemplateVersionParameter, 0)
	for _, param := range q.templateVersionParameters {
		if param.TemplateVersionID != templateVersionID {
			continue
		}
		parameters = append(parameters, param)
	}
	sort.Slice(parameters, func(i, j int) bool {
		if parameters[i].DisplayOrder != parameters[j].DisplayOrder {
			return parameters[i].DisplayOrder < parameters[j].DisplayOrder
		}
		return strings.ToLower(parameters[i].Name) < strings.ToLower(parameters[j].Name)
	})
	return parameters, nil
}

func (q *FakeQuerier) GetTemplateVersionVariables(_ context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionVariable, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	variables := make([]database.TemplateVersionVariable, 0)
	for _, variable := range q.templateVersionVariables {
		if variable.TemplateVersionID != templateVersionID {
			continue
		}
		variables = append(variables, variable)
	}
	return variables, nil
}

func (q *FakeQuerier) GetTemplateVersionsByIDs(_ context.Context, ids []uuid.UUID) ([]database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	versions := make([]database.TemplateVersion, 0)
	for _, version := range q.templateVersions {
		for _, id := range ids {
			if id == version.ID {
				versions = append(versions, q.templateVersionWithUserNoLock(version))
				break
			}
		}
	}
	if len(versions) == 0 {
		return nil, sql.ErrNoRows
	}

	return versions, nil
}

func (q *FakeQuerier) GetTemplateVersionsByTemplateID(_ context.Context, arg database.GetTemplateVersionsByTemplateIDParams) (version []database.TemplateVersion, err error) {
	if err := validateDatabaseType(arg); err != nil {
		return version, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, templateVersion := range q.templateVersions {
		if templateVersion.TemplateID.UUID != arg.TemplateID {
			continue
		}
		if arg.Archived.Valid && arg.Archived.Bool != templateVersion.Archived {
			continue
		}
		version = append(version, q.templateVersionWithUserNoLock(templateVersion))
	}

	// Database orders by created_at
	slices.SortFunc(version, func(a, b database.TemplateVersion) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			// Technically the postgres database also orders by uuid. So match
			// that behavior
			return slice.Ascending(a.ID.String(), b.ID.String())
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
	})

	if arg.AfterID != uuid.Nil {
		found := false
		for i, v := range version {
			if v.ID == arg.AfterID {
				// We want to return all users after index i.
				version = version[i+1:]
				found = true
				break
			}
		}

		// If no users after the time, then we return an empty list.
		if !found {
			return nil, sql.ErrNoRows
		}
	}

	if arg.OffsetOpt > 0 {
		if int(arg.OffsetOpt) > len(version)-1 {
			return nil, sql.ErrNoRows
		}
		version = version[arg.OffsetOpt:]
	}

	if arg.LimitOpt > 0 {
		if int(arg.LimitOpt) > len(version) {
			arg.LimitOpt = int32(len(version))
		}
		version = version[:arg.LimitOpt]
	}

	if len(version) == 0 {
		return nil, sql.ErrNoRows
	}

	return version, nil
}

func (q *FakeQuerier) GetTemplateVersionsCreatedAfter(_ context.Context, after time.Time) ([]database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	versions := make([]database.TemplateVersion, 0)
	for _, version := range q.templateVersions {
		if version.CreatedAt.After(after) {
			versions = append(versions, q.templateVersionWithUserNoLock(version))
		}
	}
	return versions, nil
}

func (q *FakeQuerier) GetTemplates(_ context.Context) ([]database.Template, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	templates := slices.Clone(q.templates)
	slices.SortFunc(templates, func(a, b database.TemplateTable) int {
		if a.Name != b.Name {
			return slice.Ascending(a.Name, b.Name)
		}
		return slice.Ascending(a.ID.String(), b.ID.String())
	})

	return q.templatesWithUserNoLock(templates), nil
}

func (q *FakeQuerier) GetTemplatesWithFilter(ctx context.Context, arg database.GetTemplatesWithFilterParams) ([]database.Template, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	return q.GetAuthorizedTemplates(ctx, arg, nil)
}

func (q *FakeQuerier) GetUnexpiredLicenses(_ context.Context) ([]database.License, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	now := time.Now()
	var results []database.License
	for _, l := range q.licenses {
		if l.Exp.After(now) {
			results = append(results, l)
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
	return results, nil
}

func (q *FakeQuerier) GetUserActivityInsights(ctx context.Context, arg database.GetUserActivityInsightsParams) ([]database.GetUserActivityInsightsRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	type uniqueKey struct {
		TemplateID uuid.UUID
		UserID     uuid.UUID
	}

	combinedStats := make(map[uniqueKey]map[time.Time]int64)

	// Get application stats
	for _, s := range q.workspaceAppStats {
		if !(((s.SessionStartedAt.After(arg.StartTime) || s.SessionStartedAt.Equal(arg.StartTime)) && s.SessionStartedAt.Before(arg.EndTime)) ||
			(s.SessionEndedAt.After(arg.StartTime) && s.SessionEndedAt.Before(arg.EndTime)) ||
			(s.SessionStartedAt.Before(arg.StartTime) && (s.SessionEndedAt.After(arg.EndTime) || s.SessionEndedAt.Equal(arg.EndTime)))) {
			continue
		}

		w, err := q.getWorkspaceByIDNoLock(ctx, s.WorkspaceID)
		if err != nil {
			return nil, err
		}

		if len(arg.TemplateIDs) > 0 && !slices.Contains(arg.TemplateIDs, w.TemplateID) {
			continue
		}

		key := uniqueKey{
			TemplateID: w.TemplateID,
			UserID:     s.UserID,
		}
		if combinedStats[key] == nil {
			combinedStats[key] = make(map[time.Time]int64)
		}

		t := s.SessionStartedAt.Truncate(time.Minute)
		if t.Before(arg.StartTime) {
			t = arg.StartTime
		}
		for t.Before(s.SessionEndedAt) && t.Before(arg.EndTime) {
			combinedStats[key][t] = 60
			t = t.Add(1 * time.Minute)
		}
	}

	// Get session stats
	for _, s := range q.workspaceAgentStats {
		if s.CreatedAt.Before(arg.StartTime) || s.CreatedAt.Equal(arg.EndTime) || s.CreatedAt.After(arg.EndTime) {
			continue
		}
		if len(arg.TemplateIDs) > 0 && !slices.Contains(arg.TemplateIDs, s.TemplateID) {
			continue
		}
		if s.ConnectionCount == 0 {
			continue
		}

		key := uniqueKey{
			TemplateID: s.TemplateID,
			UserID:     s.UserID,
		}

		if combinedStats[key] == nil {
			combinedStats[key] = make(map[time.Time]int64)
		}

		if s.SessionCountJetBrains > 0 || s.SessionCountVSCode > 0 || s.SessionCountReconnectingPTY > 0 || s.SessionCountSSH > 0 {
			t := s.CreatedAt.Truncate(time.Minute)
			combinedStats[key][t] = 60
		}
	}

	// Use temporary maps for aggregation purposes
	mUserIDTemplateIDs := map[uuid.UUID]map[uuid.UUID]struct{}{}
	mUserIDUsageSeconds := map[uuid.UUID]int64{}

	for key, times := range combinedStats {
		if mUserIDTemplateIDs[key.UserID] == nil {
			mUserIDTemplateIDs[key.UserID] = make(map[uuid.UUID]struct{})
			mUserIDUsageSeconds[key.UserID] = 0
		}

		if _, ok := mUserIDTemplateIDs[key.UserID][key.TemplateID]; !ok {
			mUserIDTemplateIDs[key.UserID][key.TemplateID] = struct{}{}
		}

		for _, t := range times {
			mUserIDUsageSeconds[key.UserID] += t
		}
	}

	userIDs := make([]uuid.UUID, 0, len(mUserIDUsageSeconds))
	for userID := range mUserIDUsageSeconds {
		userIDs = append(userIDs, userID)
	}
	sort.Slice(userIDs, func(i, j int) bool {
		return userIDs[i].String() < userIDs[j].String()
	})

	// Finally, select stats
	var rows []database.GetUserActivityInsightsRow

	for _, userID := range userIDs {
		user, err := q.getUserByIDNoLock(userID)
		if err != nil {
			return nil, err
		}

		tids := mUserIDTemplateIDs[userID]
		templateIDs := make([]uuid.UUID, 0, len(tids))
		for key := range tids {
			templateIDs = append(templateIDs, key)
		}
		sort.Slice(templateIDs, func(i, j int) bool {
			return templateIDs[i].String() < templateIDs[j].String()
		})

		row := database.GetUserActivityInsightsRow{
			UserID:       user.ID,
			Username:     user.Username,
			AvatarURL:    user.AvatarURL,
			TemplateIDs:  templateIDs,
			UsageSeconds: mUserIDUsageSeconds[userID],
		}

		rows = append(rows, row)
	}
	return rows, nil
}

func (q *FakeQuerier) GetUserByEmailOrUsername(_ context.Context, arg database.GetUserByEmailOrUsernameParams) (database.User, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.User{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, user := range q.users {
		if !user.Deleted && (strings.EqualFold(user.Email, arg.Email) || strings.EqualFold(user.Username, arg.Username)) {
			return user, nil
		}
	}
	return database.User{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetUserByID(_ context.Context, id uuid.UUID) (database.User, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getUserByIDNoLock(id)
}

func (q *FakeQuerier) GetUserCount(_ context.Context) (int64, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	existing := int64(0)
	for _, u := range q.users {
		if !u.Deleted {
			existing++
		}
	}
	return existing, nil
}

func (q *FakeQuerier) GetUserLatencyInsights(_ context.Context, arg database.GetUserLatencyInsightsParams) ([]database.GetUserLatencyInsightsRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	latenciesByUserID := make(map[uuid.UUID][]float64)
	seenTemplatesByUserID := make(map[uuid.UUID]map[uuid.UUID]struct{})
	for _, s := range q.workspaceAgentStats {
		if len(arg.TemplateIDs) > 0 && !slices.Contains(arg.TemplateIDs, s.TemplateID) {
			continue
		}
		if !arg.StartTime.Equal(s.CreatedAt) && (s.CreatedAt.Before(arg.StartTime) || s.CreatedAt.After(arg.EndTime)) {
			continue
		}
		if s.ConnectionCount == 0 {
			continue
		}
		if s.ConnectionMedianLatencyMS <= 0 {
			continue
		}

		latenciesByUserID[s.UserID] = append(latenciesByUserID[s.UserID], s.ConnectionMedianLatencyMS)
		if seenTemplatesByUserID[s.UserID] == nil {
			seenTemplatesByUserID[s.UserID] = make(map[uuid.UUID]struct{})
		}
		seenTemplatesByUserID[s.UserID][s.TemplateID] = struct{}{}
	}

	tryPercentile := func(fs []float64, p float64) float64 {
		if len(fs) == 0 {
			return -1
		}
		sort.Float64s(fs)
		return fs[int(float64(len(fs))*p/100)]
	}

	var rows []database.GetUserLatencyInsightsRow
	for userID, latencies := range latenciesByUserID {
		sort.Float64s(latencies)
		templateIDSet := seenTemplatesByUserID[userID]
		templateIDs := make([]uuid.UUID, 0, len(templateIDSet))
		for templateID := range templateIDSet {
			templateIDs = append(templateIDs, templateID)
		}
		slices.SortFunc(templateIDs, func(a, b uuid.UUID) int {
			return slice.Ascending(a.String(), b.String())
		})
		user, err := q.getUserByIDNoLock(userID)
		if err != nil {
			return nil, err
		}
		row := database.GetUserLatencyInsightsRow{
			UserID:                       userID,
			Username:                     user.Username,
			AvatarURL:                    user.AvatarURL,
			TemplateIDs:                  templateIDs,
			WorkspaceConnectionLatency50: tryPercentile(latencies, 50),
			WorkspaceConnectionLatency95: tryPercentile(latencies, 95),
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b database.GetUserLatencyInsightsRow) int {
		return slice.Ascending(a.UserID.String(), b.UserID.String())
	})

	return rows, nil
}

func (q *FakeQuerier) GetUserLinkByLinkedID(_ context.Context, id string) (database.UserLink, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, link := range q.userLinks {
		if link.LinkedID == id {
			return link, nil
		}
	}
	return database.UserLink{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetUserLinkByUserIDLoginType(_ context.Context, params database.GetUserLinkByUserIDLoginTypeParams) (database.UserLink, error) {
	if err := validateDatabaseType(params); err != nil {
		return database.UserLink{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, link := range q.userLinks {
		if link.UserID == params.UserID && link.LoginType == params.LoginType {
			return link, nil
		}
	}
	return database.UserLink{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetUserLinksByUserID(_ context.Context, userID uuid.UUID) ([]database.UserLink, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	uls := make([]database.UserLink, 0)
	for _, ul := range q.userLinks {
		if ul.UserID == userID {
			uls = append(uls, ul)
		}
	}
	return uls, nil
}

func (q *FakeQuerier) GetUsers(_ context.Context, params database.GetUsersParams) ([]database.GetUsersRow, error) {
	if err := validateDatabaseType(params); err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// Avoid side-effect of sorting.
	users := make([]database.User, len(q.users))
	copy(users, q.users)

	// Database orders by username
	slices.SortFunc(users, func(a, b database.User) int {
		return slice.Ascending(strings.ToLower(a.Username), strings.ToLower(b.Username))
	})

	// Filter out deleted since they should never be returned..
	tmp := make([]database.User, 0, len(users))
	for _, user := range users {
		if !user.Deleted {
			tmp = append(tmp, user)
		}
	}
	users = tmp

	if params.AfterID != uuid.Nil {
		found := false
		for i, v := range users {
			if v.ID == params.AfterID {
				// We want to return all users after index i.
				users = users[i+1:]
				found = true
				break
			}
		}

		// If no users after the time, then we return an empty list.
		if !found {
			return []database.GetUsersRow{}, nil
		}
	}

	if params.Search != "" {
		tmp := make([]database.User, 0, len(users))
		for i, user := range users {
			if strings.Contains(strings.ToLower(user.Email), strings.ToLower(params.Search)) {
				tmp = append(tmp, users[i])
			} else if strings.Contains(strings.ToLower(user.Username), strings.ToLower(params.Search)) {
				tmp = append(tmp, users[i])
			}
		}
		users = tmp
	}

	if len(params.Status) > 0 {
		usersFilteredByStatus := make([]database.User, 0, len(users))
		for i, user := range users {
			if slice.ContainsCompare(params.Status, user.Status, func(a, b database.UserStatus) bool {
				return strings.EqualFold(string(a), string(b))
			}) {
				usersFilteredByStatus = append(usersFilteredByStatus, users[i])
			}
		}
		users = usersFilteredByStatus
	}

	if len(params.RbacRole) > 0 && !slice.Contains(params.RbacRole, rbac.RoleMember()) {
		usersFilteredByRole := make([]database.User, 0, len(users))
		for i, user := range users {
			if slice.OverlapCompare(params.RbacRole, user.RBACRoles, strings.EqualFold) {
				usersFilteredByRole = append(usersFilteredByRole, users[i])
			}
		}
		users = usersFilteredByRole
	}

	if !params.LastSeenBefore.IsZero() {
		usersFilteredByLastSeen := make([]database.User, 0, len(users))
		for i, user := range users {
			if user.LastSeenAt.Before(params.LastSeenBefore) {
				usersFilteredByLastSeen = append(usersFilteredByLastSeen, users[i])
			}
		}
		users = usersFilteredByLastSeen
	}

	if !params.LastSeenAfter.IsZero() {
		usersFilteredByLastSeen := make([]database.User, 0, len(users))
		for i, user := range users {
			if user.LastSeenAt.After(params.LastSeenAfter) {
				usersFilteredByLastSeen = append(usersFilteredByLastSeen, users[i])
			}
		}
		users = usersFilteredByLastSeen
	}

	beforePageCount := len(users)

	if params.OffsetOpt > 0 {
		if int(params.OffsetOpt) > len(users)-1 {
			return []database.GetUsersRow{}, nil
		}
		users = users[params.OffsetOpt:]
	}

	if params.LimitOpt > 0 {
		if int(params.LimitOpt) > len(users) {
			params.LimitOpt = int32(len(users))
		}
		users = users[:params.LimitOpt]
	}

	return convertUsers(users, int64(beforePageCount)), nil
}

func (q *FakeQuerier) GetUsersByIDs(_ context.Context, ids []uuid.UUID) ([]database.User, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	users := make([]database.User, 0)
	for _, user := range q.users {
		for _, id := range ids {
			if user.ID != id {
				continue
			}
			users = append(users, user)
		}
	}
	return users, nil
}

func (q *FakeQuerier) GetWorkspaceAgentAndOwnerByAuthToken(_ context.Context, authToken uuid.UUID) (database.GetWorkspaceAgentAndOwnerByAuthTokenRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// map of build number -> row
	rows := make(map[int32]database.GetWorkspaceAgentAndOwnerByAuthTokenRow)

	// We want to return the latest build number
	var latestBuildNumber int32

	for _, agt := range q.workspaceAgents {
		if agt.AuthToken != authToken {
			continue
		}
		// get the related workspace and user
		for _, res := range q.workspaceResources {
			if agt.ResourceID != res.ID {
				continue
			}
			for _, build := range q.workspaceBuilds {
				if build.JobID != res.JobID {
					continue
				}
				for _, ws := range q.workspaces {
					if build.WorkspaceID != ws.ID {
						continue
					}
					var row database.GetWorkspaceAgentAndOwnerByAuthTokenRow
					row.WorkspaceID = ws.ID
					usr, err := q.getUserByIDNoLock(ws.OwnerID)
					if err != nil {
						return database.GetWorkspaceAgentAndOwnerByAuthTokenRow{}, sql.ErrNoRows
					}
					row.OwnerID = usr.ID
					row.OwnerRoles = append(usr.RBACRoles, "member")
					// We also need to get org roles for the user
					row.OwnerName = usr.Username
					row.WorkspaceAgent = agt
					for _, mem := range q.organizationMembers {
						if mem.UserID == usr.ID {
							row.OwnerRoles = append(row.OwnerRoles, fmt.Sprintf("organization-member:%s", mem.OrganizationID.String()))
						}
					}
					// And group memberships
					for _, groupMem := range q.groupMembers {
						if groupMem.UserID == usr.ID {
							row.OwnerGroups = append(row.OwnerGroups, groupMem.GroupID.String())
						}
					}

					// Keep track of the latest build number
					rows[build.BuildNumber] = row
					if build.BuildNumber > latestBuildNumber {
						latestBuildNumber = build.BuildNumber
					}
				}
			}
		}
	}

	if len(rows) == 0 {
		return database.GetWorkspaceAgentAndOwnerByAuthTokenRow{}, sql.ErrNoRows
	}

	// Return the row related to the latest build
	return rows[latestBuildNumber], nil
}

func (q *FakeQuerier) GetWorkspaceAgentByID(ctx context.Context, id uuid.UUID) (database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getWorkspaceAgentByIDNoLock(ctx, id)
}

func (q *FakeQuerier) GetWorkspaceAgentByInstanceID(_ context.Context, instanceID string) (database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// The schema sorts this by created at, so we iterate the array backwards.
	for i := len(q.workspaceAgents) - 1; i >= 0; i-- {
		agent := q.workspaceAgents[i]
		if agent.AuthInstanceID.Valid && agent.AuthInstanceID.String == instanceID {
			return agent, nil
		}
	}
	return database.WorkspaceAgent{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceAgentLifecycleStateByID(ctx context.Context, id uuid.UUID) (database.GetWorkspaceAgentLifecycleStateByIDRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	agent, err := q.getWorkspaceAgentByIDNoLock(ctx, id)
	if err != nil {
		return database.GetWorkspaceAgentLifecycleStateByIDRow{}, err
	}
	return database.GetWorkspaceAgentLifecycleStateByIDRow{
		LifecycleState: agent.LifecycleState,
		StartedAt:      agent.StartedAt,
		ReadyAt:        agent.ReadyAt,
	}, nil
}

func (q *FakeQuerier) GetWorkspaceAgentLogSourcesByAgentIDs(_ context.Context, ids []uuid.UUID) ([]database.WorkspaceAgentLogSource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	logSources := make([]database.WorkspaceAgentLogSource, 0)
	for _, logSource := range q.workspaceAgentLogSources {
		for _, id := range ids {
			if logSource.WorkspaceAgentID == id {
				logSources = append(logSources, logSource)
				break
			}
		}
	}
	return logSources, nil
}

func (q *FakeQuerier) GetWorkspaceAgentLogsAfter(_ context.Context, arg database.GetWorkspaceAgentLogsAfterParams) ([]database.WorkspaceAgentLog, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	logs := []database.WorkspaceAgentLog{}
	for _, log := range q.workspaceAgentLogs {
		if log.AgentID != arg.AgentID {
			continue
		}
		if arg.CreatedAfter != 0 && log.ID <= arg.CreatedAfter {
			continue
		}
		logs = append(logs, log)
	}
	return logs, nil
}

func (q *FakeQuerier) GetWorkspaceAgentMetadata(_ context.Context, arg database.GetWorkspaceAgentMetadataParams) ([]database.WorkspaceAgentMetadatum, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	metadata := make([]database.WorkspaceAgentMetadatum, 0)
	for _, m := range q.workspaceAgentMetadata {
		if m.WorkspaceAgentID == arg.WorkspaceAgentID {
			if len(arg.Keys) > 0 && !slices.Contains(arg.Keys, m.Key) {
				continue
			}
			metadata = append(metadata, m)
		}
	}
	return metadata, nil
}

func (q *FakeQuerier) GetWorkspaceAgentScriptsByAgentIDs(_ context.Context, ids []uuid.UUID) ([]database.WorkspaceAgentScript, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	scripts := make([]database.WorkspaceAgentScript, 0)
	for _, script := range q.workspaceAgentScripts {
		for _, id := range ids {
			if script.WorkspaceAgentID == id {
				scripts = append(scripts, script)
				break
			}
		}
	}
	return scripts, nil
}

func (q *FakeQuerier) GetWorkspaceAgentStats(_ context.Context, createdAfter time.Time) ([]database.GetWorkspaceAgentStatsRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	agentStatsCreatedAfter := make([]database.WorkspaceAgentStat, 0)
	for _, agentStat := range q.workspaceAgentStats {
		if agentStat.CreatedAt.After(createdAfter) || agentStat.CreatedAt.Equal(createdAfter) {
			agentStatsCreatedAfter = append(agentStatsCreatedAfter, agentStat)
		}
	}

	latestAgentStats := map[uuid.UUID]database.WorkspaceAgentStat{}
	for _, agentStat := range q.workspaceAgentStats {
		if agentStat.CreatedAt.After(createdAfter) || agentStat.CreatedAt.Equal(createdAfter) {
			latestAgentStats[agentStat.AgentID] = agentStat
		}
	}

	statByAgent := map[uuid.UUID]database.GetWorkspaceAgentStatsRow{}
	for agentID, agentStat := range latestAgentStats {
		stat := statByAgent[agentID]
		stat.AgentID = agentStat.AgentID
		stat.TemplateID = agentStat.TemplateID
		stat.UserID = agentStat.UserID
		stat.WorkspaceID = agentStat.WorkspaceID
		stat.SessionCountVSCode += agentStat.SessionCountVSCode
		stat.SessionCountJetBrains += agentStat.SessionCountJetBrains
		stat.SessionCountReconnectingPTY += agentStat.SessionCountReconnectingPTY
		stat.SessionCountSSH += agentStat.SessionCountSSH
		statByAgent[stat.AgentID] = stat
	}

	latenciesByAgent := map[uuid.UUID][]float64{}
	minimumDateByAgent := map[uuid.UUID]time.Time{}
	for _, agentStat := range agentStatsCreatedAfter {
		if agentStat.ConnectionMedianLatencyMS <= 0 {
			continue
		}
		stat := statByAgent[agentStat.AgentID]
		minimumDate := minimumDateByAgent[agentStat.AgentID]
		if agentStat.CreatedAt.Before(minimumDate) || minimumDate.IsZero() {
			minimumDateByAgent[agentStat.AgentID] = agentStat.CreatedAt
		}
		stat.WorkspaceRxBytes += agentStat.RxBytes
		stat.WorkspaceTxBytes += agentStat.TxBytes
		statByAgent[agentStat.AgentID] = stat
		latenciesByAgent[agentStat.AgentID] = append(latenciesByAgent[agentStat.AgentID], agentStat.ConnectionMedianLatencyMS)
	}

	tryPercentile := func(fs []float64, p float64) float64 {
		if len(fs) == 0 {
			return -1
		}
		sort.Float64s(fs)
		return fs[int(float64(len(fs))*p/100)]
	}

	for _, stat := range statByAgent {
		stat.AggregatedFrom = minimumDateByAgent[stat.AgentID]
		statByAgent[stat.AgentID] = stat

		latencies, ok := latenciesByAgent[stat.AgentID]
		if !ok {
			continue
		}
		stat.WorkspaceConnectionLatency50 = tryPercentile(latencies, 50)
		stat.WorkspaceConnectionLatency95 = tryPercentile(latencies, 95)
		statByAgent[stat.AgentID] = stat
	}

	stats := make([]database.GetWorkspaceAgentStatsRow, 0, len(statByAgent))
	for _, agent := range statByAgent {
		stats = append(stats, agent)
	}
	return stats, nil
}

func (q *FakeQuerier) GetWorkspaceAgentStatsAndLabels(ctx context.Context, createdAfter time.Time) ([]database.GetWorkspaceAgentStatsAndLabelsRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	agentStatsCreatedAfter := make([]database.WorkspaceAgentStat, 0)
	latestAgentStats := map[uuid.UUID]database.WorkspaceAgentStat{}

	for _, agentStat := range q.workspaceAgentStats {
		if agentStat.CreatedAt.After(createdAfter) {
			agentStatsCreatedAfter = append(agentStatsCreatedAfter, agentStat)
			latestAgentStats[agentStat.AgentID] = agentStat
		}
	}

	statByAgent := map[uuid.UUID]database.GetWorkspaceAgentStatsAndLabelsRow{}

	// Session and connection metrics
	for _, agentStat := range latestAgentStats {
		stat := statByAgent[agentStat.AgentID]
		stat.SessionCountVSCode += agentStat.SessionCountVSCode
		stat.SessionCountJetBrains += agentStat.SessionCountJetBrains
		stat.SessionCountReconnectingPTY += agentStat.SessionCountReconnectingPTY
		stat.SessionCountSSH += agentStat.SessionCountSSH
		stat.ConnectionCount += agentStat.ConnectionCount
		if agentStat.ConnectionMedianLatencyMS >= 0 && stat.ConnectionMedianLatencyMS < agentStat.ConnectionMedianLatencyMS {
			stat.ConnectionMedianLatencyMS = agentStat.ConnectionMedianLatencyMS
		}
		statByAgent[agentStat.AgentID] = stat
	}

	// Tx, Rx metrics
	for _, agentStat := range agentStatsCreatedAfter {
		stat := statByAgent[agentStat.AgentID]
		stat.RxBytes += agentStat.RxBytes
		stat.TxBytes += agentStat.TxBytes
		statByAgent[agentStat.AgentID] = stat
	}

	// Labels
	for _, agentStat := range agentStatsCreatedAfter {
		stat := statByAgent[agentStat.AgentID]

		user, err := q.getUserByIDNoLock(agentStat.UserID)
		if err != nil {
			return nil, err
		}

		stat.Username = user.Username

		workspace, err := q.getWorkspaceByIDNoLock(ctx, agentStat.WorkspaceID)
		if err != nil {
			return nil, err
		}
		stat.WorkspaceName = workspace.Name

		agent, err := q.getWorkspaceAgentByIDNoLock(ctx, agentStat.AgentID)
		if err != nil {
			return nil, err
		}
		stat.AgentName = agent.Name

		statByAgent[agentStat.AgentID] = stat
	}

	stats := make([]database.GetWorkspaceAgentStatsAndLabelsRow, 0, len(statByAgent))
	for _, agent := range statByAgent {
		stats = append(stats, agent)
	}
	return stats, nil
}

func (q *FakeQuerier) GetWorkspaceAgentsByResourceIDs(ctx context.Context, resourceIDs []uuid.UUID) ([]database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getWorkspaceAgentsByResourceIDsNoLock(ctx, resourceIDs)
}

func (q *FakeQuerier) GetWorkspaceAgentsCreatedAfter(_ context.Context, after time.Time) ([]database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaceAgents := make([]database.WorkspaceAgent, 0)
	for _, agent := range q.workspaceAgents {
		if agent.CreatedAt.After(after) {
			workspaceAgents = append(workspaceAgents, agent)
		}
	}
	return workspaceAgents, nil
}

func (q *FakeQuerier) GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx context.Context, workspaceID uuid.UUID) ([]database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// Get latest build for workspace.
	workspaceBuild, err := q.getLatestWorkspaceBuildByWorkspaceIDNoLock(ctx, workspaceID)
	if err != nil {
		return nil, xerrors.Errorf("get latest workspace build: %w", err)
	}

	// Get resources for build.
	resources, err := q.getWorkspaceResourcesByJobIDNoLock(ctx, workspaceBuild.JobID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace resources: %w", err)
	}
	if len(resources) == 0 {
		return []database.WorkspaceAgent{}, nil
	}

	resourceIDs := make([]uuid.UUID, len(resources))
	for i, resource := range resources {
		resourceIDs[i] = resource.ID
	}

	agents, err := q.getWorkspaceAgentsByResourceIDsNoLock(ctx, resourceIDs)
	if err != nil {
		return nil, xerrors.Errorf("get workspace agents: %w", err)
	}

	return agents, nil
}

func (q *FakeQuerier) GetWorkspaceAppByAgentIDAndSlug(ctx context.Context, arg database.GetWorkspaceAppByAgentIDAndSlugParams) (database.WorkspaceApp, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.WorkspaceApp{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getWorkspaceAppByAgentIDAndSlugNoLock(ctx, arg)
}

func (q *FakeQuerier) GetWorkspaceAppsByAgentID(_ context.Context, id uuid.UUID) ([]database.WorkspaceApp, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	apps := make([]database.WorkspaceApp, 0)
	for _, app := range q.workspaceApps {
		if app.AgentID == id {
			apps = append(apps, app)
		}
	}
	return apps, nil
}

func (q *FakeQuerier) GetWorkspaceAppsByAgentIDs(_ context.Context, ids []uuid.UUID) ([]database.WorkspaceApp, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	apps := make([]database.WorkspaceApp, 0)
	for _, app := range q.workspaceApps {
		for _, id := range ids {
			if app.AgentID == id {
				apps = append(apps, app)
				break
			}
		}
	}
	return apps, nil
}

func (q *FakeQuerier) GetWorkspaceAppsCreatedAfter(_ context.Context, after time.Time) ([]database.WorkspaceApp, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	apps := make([]database.WorkspaceApp, 0)
	for _, app := range q.workspaceApps {
		if app.CreatedAt.After(after) {
			apps = append(apps, app)
		}
	}
	return apps, nil
}

func (q *FakeQuerier) GetWorkspaceBuildByID(ctx context.Context, id uuid.UUID) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getWorkspaceBuildByIDNoLock(ctx, id)
}

func (q *FakeQuerier) GetWorkspaceBuildByJobID(_ context.Context, jobID uuid.UUID) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, build := range q.workspaceBuilds {
		if build.JobID == jobID {
			return q.workspaceBuildWithUserNoLock(build), nil
		}
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceBuildByWorkspaceIDAndBuildNumber(_ context.Context, arg database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams) (database.WorkspaceBuild, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.WorkspaceBuild{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, workspaceBuild := range q.workspaceBuilds {
		if workspaceBuild.WorkspaceID != arg.WorkspaceID {
			continue
		}
		if workspaceBuild.BuildNumber != arg.BuildNumber {
			continue
		}
		return q.workspaceBuildWithUserNoLock(workspaceBuild), nil
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceBuildParameters(_ context.Context, workspaceBuildID uuid.UUID) ([]database.WorkspaceBuildParameter, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	params := make([]database.WorkspaceBuildParameter, 0)
	for _, param := range q.workspaceBuildParameters {
		if param.WorkspaceBuildID != workspaceBuildID {
			continue
		}
		params = append(params, param)
	}
	return params, nil
}

func (q *FakeQuerier) GetWorkspaceBuildsByWorkspaceID(_ context.Context,
	params database.GetWorkspaceBuildsByWorkspaceIDParams,
) ([]database.WorkspaceBuild, error) {
	if err := validateDatabaseType(params); err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	history := make([]database.WorkspaceBuild, 0)
	for _, workspaceBuild := range q.workspaceBuilds {
		if workspaceBuild.CreatedAt.Before(params.Since) {
			continue
		}
		if workspaceBuild.WorkspaceID == params.WorkspaceID {
			history = append(history, q.workspaceBuildWithUserNoLock(workspaceBuild))
		}
	}

	// Order by build_number
	slices.SortFunc(history, func(a, b database.WorkspaceBuild) int {
		return slice.Descending(a.BuildNumber, b.BuildNumber)
	})

	if params.AfterID != uuid.Nil {
		found := false
		for i, v := range history {
			if v.ID == params.AfterID {
				// We want to return all builds after index i.
				history = history[i+1:]
				found = true
				break
			}
		}

		// If no builds after the time, then we return an empty list.
		if !found {
			return nil, sql.ErrNoRows
		}
	}

	if params.OffsetOpt > 0 {
		if int(params.OffsetOpt) > len(history)-1 {
			return nil, sql.ErrNoRows
		}
		history = history[params.OffsetOpt:]
	}

	if params.LimitOpt > 0 {
		if int(params.LimitOpt) > len(history) {
			params.LimitOpt = int32(len(history))
		}
		history = history[:params.LimitOpt]
	}

	if len(history) == 0 {
		return nil, sql.ErrNoRows
	}
	return history, nil
}

func (q *FakeQuerier) GetWorkspaceBuildsCreatedAfter(_ context.Context, after time.Time) ([]database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaceBuilds := make([]database.WorkspaceBuild, 0)
	for _, workspaceBuild := range q.workspaceBuilds {
		if workspaceBuild.CreatedAt.After(after) {
			workspaceBuilds = append(workspaceBuilds, q.workspaceBuildWithUserNoLock(workspaceBuild))
		}
	}
	return workspaceBuilds, nil
}

func (q *FakeQuerier) GetWorkspaceByAgentID(ctx context.Context, agentID uuid.UUID) (database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getWorkspaceByAgentIDNoLock(ctx, agentID)
}

func (q *FakeQuerier) GetWorkspaceByID(ctx context.Context, id uuid.UUID) (database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getWorkspaceByIDNoLock(ctx, id)
}

func (q *FakeQuerier) GetWorkspaceByOwnerIDAndName(_ context.Context, arg database.GetWorkspaceByOwnerIDAndNameParams) (database.Workspace, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Workspace{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var found *database.Workspace
	for _, workspace := range q.workspaces {
		workspace := workspace
		if workspace.OwnerID != arg.OwnerID {
			continue
		}
		if !strings.EqualFold(workspace.Name, arg.Name) {
			continue
		}
		if workspace.Deleted != arg.Deleted {
			continue
		}

		// Return the most recent workspace with the given name
		if found == nil || workspace.CreatedAt.After(found.CreatedAt) {
			found = &workspace
		}
	}
	if found != nil {
		return *found, nil
	}
	return database.Workspace{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceByWorkspaceAppID(_ context.Context, workspaceAppID uuid.UUID) (database.Workspace, error) {
	if err := validateDatabaseType(workspaceAppID); err != nil {
		return database.Workspace{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, workspaceApp := range q.workspaceApps {
		workspaceApp := workspaceApp
		if workspaceApp.ID == workspaceAppID {
			return q.getWorkspaceByAgentIDNoLock(context.Background(), workspaceApp.AgentID)
		}
	}
	return database.Workspace{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceProxies(_ context.Context) ([]database.WorkspaceProxy, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	cpy := make([]database.WorkspaceProxy, 0, len(q.workspaceProxies))

	for _, p := range q.workspaceProxies {
		if !p.Deleted {
			cpy = append(cpy, p)
		}
	}
	return cpy, nil
}

func (q *FakeQuerier) GetWorkspaceProxyByHostname(_ context.Context, params database.GetWorkspaceProxyByHostnameParams) (database.WorkspaceProxy, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// Return zero rows if this is called with a non-sanitized hostname. The SQL
	// version of this query does the same thing.
	if !validProxyByHostnameRegex.MatchString(params.Hostname) {
		return database.WorkspaceProxy{}, sql.ErrNoRows
	}

	// This regex matches the SQL version.
	accessURLRegex := regexp.MustCompile(`[^:]*://` + regexp.QuoteMeta(params.Hostname) + `([:/]?.)*`)

	for _, proxy := range q.workspaceProxies {
		if proxy.Deleted {
			continue
		}
		if params.AllowAccessUrl && accessURLRegex.MatchString(proxy.Url) {
			return proxy, nil
		}

		// Compile the app hostname regex. This is slow sadly.
		if params.AllowWildcardHostname {
			wildcardRegexp, err := httpapi.CompileHostnamePattern(proxy.WildcardHostname)
			if err != nil {
				return database.WorkspaceProxy{}, xerrors.Errorf("compile hostname pattern %q for proxy %q (%s): %w", proxy.WildcardHostname, proxy.Name, proxy.ID.String(), err)
			}
			if _, ok := httpapi.ExecuteHostnamePattern(wildcardRegexp, params.Hostname); ok {
				return proxy, nil
			}
		}
	}

	return database.WorkspaceProxy{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceProxyByID(_ context.Context, id uuid.UUID) (database.WorkspaceProxy, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, proxy := range q.workspaceProxies {
		if proxy.ID == id {
			return proxy, nil
		}
	}
	return database.WorkspaceProxy{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceProxyByName(_ context.Context, name string) (database.WorkspaceProxy, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, proxy := range q.workspaceProxies {
		if proxy.Deleted {
			continue
		}
		if proxy.Name == name {
			return proxy, nil
		}
	}
	return database.WorkspaceProxy{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceResourceByID(_ context.Context, id uuid.UUID) (database.WorkspaceResource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, resource := range q.workspaceResources {
		if resource.ID == id {
			return resource, nil
		}
	}
	return database.WorkspaceResource{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceResourceMetadataByResourceIDs(_ context.Context, ids []uuid.UUID) ([]database.WorkspaceResourceMetadatum, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	metadata := make([]database.WorkspaceResourceMetadatum, 0)
	for _, metadatum := range q.workspaceResourceMetadata {
		for _, id := range ids {
			if metadatum.WorkspaceResourceID == id {
				metadata = append(metadata, metadatum)
			}
		}
	}
	return metadata, nil
}

func (q *FakeQuerier) GetWorkspaceResourceMetadataCreatedAfter(ctx context.Context, after time.Time) ([]database.WorkspaceResourceMetadatum, error) {
	resources, err := q.GetWorkspaceResourcesCreatedAfter(ctx, after)
	if err != nil {
		return nil, err
	}
	resourceIDs := map[uuid.UUID]struct{}{}
	for _, resource := range resources {
		resourceIDs[resource.ID] = struct{}{}
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	metadata := make([]database.WorkspaceResourceMetadatum, 0)
	for _, m := range q.workspaceResourceMetadata {
		_, ok := resourceIDs[m.WorkspaceResourceID]
		if !ok {
			continue
		}
		metadata = append(metadata, m)
	}
	return metadata, nil
}

func (q *FakeQuerier) GetWorkspaceResourcesByJobID(ctx context.Context, jobID uuid.UUID) ([]database.WorkspaceResource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getWorkspaceResourcesByJobIDNoLock(ctx, jobID)
}

func (q *FakeQuerier) GetWorkspaceResourcesByJobIDs(_ context.Context, jobIDs []uuid.UUID) ([]database.WorkspaceResource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	resources := make([]database.WorkspaceResource, 0)
	for _, resource := range q.workspaceResources {
		for _, jobID := range jobIDs {
			if resource.JobID != jobID {
				continue
			}
			resources = append(resources, resource)
		}
	}
	return resources, nil
}

func (q *FakeQuerier) GetWorkspaceResourcesCreatedAfter(_ context.Context, after time.Time) ([]database.WorkspaceResource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	resources := make([]database.WorkspaceResource, 0)
	for _, resource := range q.workspaceResources {
		if resource.CreatedAt.After(after) {
			resources = append(resources, resource)
		}
	}
	return resources, nil
}

func (q *FakeQuerier) GetWorkspaces(ctx context.Context, arg database.GetWorkspacesParams) ([]database.GetWorkspacesRow, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	// A nil auth filter means no auth filter.
	workspaceRows, err := q.GetAuthorizedWorkspaces(ctx, arg, nil)
	return workspaceRows, err
}

func (q *FakeQuerier) GetWorkspacesEligibleForTransition(ctx context.Context, now time.Time) ([]database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaces := []database.Workspace{}
	for _, workspace := range q.workspaces {
		build, err := q.getLatestWorkspaceBuildByWorkspaceIDNoLock(ctx, workspace.ID)
		if err != nil {
			return nil, err
		}

		if build.Transition == database.WorkspaceTransitionStart &&
			!build.Deadline.IsZero() &&
			build.Deadline.Before(now) &&
			!workspace.DormantAt.Valid {
			workspaces = append(workspaces, workspace)
			continue
		}

		if build.Transition == database.WorkspaceTransitionStop &&
			workspace.AutostartSchedule.Valid &&
			!workspace.DormantAt.Valid {
			workspaces = append(workspaces, workspace)
			continue
		}

		job, err := q.getProvisionerJobByIDNoLock(ctx, build.JobID)
		if err != nil {
			return nil, xerrors.Errorf("get provisioner job by ID: %w", err)
		}
		if codersdk.ProvisionerJobStatus(job.JobStatus) == codersdk.ProvisionerJobFailed {
			workspaces = append(workspaces, workspace)
			continue
		}

		template, err := q.getTemplateByIDNoLock(ctx, workspace.TemplateID)
		if err != nil {
			return nil, xerrors.Errorf("get template by ID: %w", err)
		}
		if !workspace.DormantAt.Valid && template.TimeTilDormant > 0 {
			workspaces = append(workspaces, workspace)
			continue
		}
		if workspace.DormantAt.Valid && template.TimeTilDormantAutoDelete > 0 {
			workspaces = append(workspaces, workspace)
			continue
		}
	}

	return workspaces, nil
}

func (q *FakeQuerier) InsertAPIKey(_ context.Context, arg database.InsertAPIKeyParams) (database.APIKey, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.APIKey{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	if arg.LifetimeSeconds == 0 {
		arg.LifetimeSeconds = 86400
	}

	for _, u := range q.users {
		if u.ID == arg.UserID && u.Deleted {
			return database.APIKey{}, xerrors.Errorf("refusing to create APIKey for deleted user")
		}
	}

	//nolint:gosimple
	key := database.APIKey{
		ID:              arg.ID,
		LifetimeSeconds: arg.LifetimeSeconds,
		HashedSecret:    arg.HashedSecret,
		IPAddress:       arg.IPAddress,
		UserID:          arg.UserID,
		ExpiresAt:       arg.ExpiresAt,
		CreatedAt:       arg.CreatedAt,
		UpdatedAt:       arg.UpdatedAt,
		LastUsed:        arg.LastUsed,
		LoginType:       arg.LoginType,
		Scope:           arg.Scope,
		TokenName:       arg.TokenName,
	}
	q.apiKeys = append(q.apiKeys, key)
	return key, nil
}

func (q *FakeQuerier) InsertAllUsersGroup(ctx context.Context, orgID uuid.UUID) (database.Group, error) {
	return q.InsertGroup(ctx, database.InsertGroupParams{
		ID:             orgID,
		Name:           database.EveryoneGroup,
		DisplayName:    "",
		OrganizationID: orgID,
		AvatarURL:      "",
		QuotaAllowance: 0,
	})
}

func (q *FakeQuerier) InsertAuditLog(_ context.Context, arg database.InsertAuditLogParams) (database.AuditLog, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.AuditLog{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	alog := database.AuditLog(arg)

	q.auditLogs = append(q.auditLogs, alog)
	slices.SortFunc(q.auditLogs, func(a, b database.AuditLog) int {
		if a.Time.Before(b.Time) {
			return -1
		} else if a.Time.Equal(b.Time) {
			return 0
		} else {
			return 1
		}
	})

	return alog, nil
}

func (q *FakeQuerier) InsertDBCryptKey(_ context.Context, arg database.InsertDBCryptKeyParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	for _, key := range q.dbcryptKeys {
		if key.Number == arg.Number {
			return errDuplicateKey
		}
	}

	q.dbcryptKeys = append(q.dbcryptKeys, database.DBCryptKey{
		Number:          arg.Number,
		ActiveKeyDigest: sql.NullString{String: arg.ActiveKeyDigest, Valid: true},
		Test:            arg.Test,
	})
	return nil
}

func (q *FakeQuerier) InsertDERPMeshKey(_ context.Context, id string) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.derpMeshKey = id
	return nil
}

func (q *FakeQuerier) InsertDeploymentID(_ context.Context, id string) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.deploymentID = id
	return nil
}

func (q *FakeQuerier) InsertExternalAuthLink(_ context.Context, arg database.InsertExternalAuthLinkParams) (database.ExternalAuthLink, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.ExternalAuthLink{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()
	// nolint:gosimple
	gitAuthLink := database.ExternalAuthLink{
		ProviderID:             arg.ProviderID,
		UserID:                 arg.UserID,
		CreatedAt:              arg.CreatedAt,
		UpdatedAt:              arg.UpdatedAt,
		OAuthAccessToken:       arg.OAuthAccessToken,
		OAuthAccessTokenKeyID:  arg.OAuthAccessTokenKeyID,
		OAuthRefreshToken:      arg.OAuthRefreshToken,
		OAuthRefreshTokenKeyID: arg.OAuthRefreshTokenKeyID,
		OAuthExpiry:            arg.OAuthExpiry,
		OAuthExtra:             arg.OAuthExtra,
	}
	q.externalAuthLinks = append(q.externalAuthLinks, gitAuthLink)
	return gitAuthLink, nil
}

func (q *FakeQuerier) InsertFile(_ context.Context, arg database.InsertFileParams) (database.File, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.File{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	file := database.File{
		ID:        arg.ID,
		Hash:      arg.Hash,
		CreatedAt: arg.CreatedAt,
		CreatedBy: arg.CreatedBy,
		Mimetype:  arg.Mimetype,
		Data:      arg.Data,
	}
	q.files = append(q.files, file)
	return file, nil
}

func (q *FakeQuerier) InsertGitSSHKey(_ context.Context, arg database.InsertGitSSHKeyParams) (database.GitSSHKey, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.GitSSHKey{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	gitSSHKey := database.GitSSHKey{
		UserID:     arg.UserID,
		CreatedAt:  arg.CreatedAt,
		UpdatedAt:  arg.UpdatedAt,
		PrivateKey: arg.PrivateKey,
		PublicKey:  arg.PublicKey,
	}
	q.gitSSHKey = append(q.gitSSHKey, gitSSHKey)
	return gitSSHKey, nil
}

func (q *FakeQuerier) InsertGroup(_ context.Context, arg database.InsertGroupParams) (database.Group, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Group{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, group := range q.groups {
		if group.OrganizationID == arg.OrganizationID &&
			group.Name == arg.Name {
			return database.Group{}, errDuplicateKey
		}
	}

	//nolint:gosimple
	group := database.Group{
		ID:             arg.ID,
		Name:           arg.Name,
		DisplayName:    arg.DisplayName,
		OrganizationID: arg.OrganizationID,
		AvatarURL:      arg.AvatarURL,
		QuotaAllowance: arg.QuotaAllowance,
		Source:         database.GroupSourceUser,
	}

	q.groups = append(q.groups, group)

	return group, nil
}

func (q *FakeQuerier) InsertGroupMember(_ context.Context, arg database.InsertGroupMemberParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, member := range q.groupMembers {
		if member.GroupID == arg.GroupID &&
			member.UserID == arg.UserID {
			return errDuplicateKey
		}
	}

	//nolint:gosimple
	q.groupMembers = append(q.groupMembers, database.GroupMember{
		GroupID: arg.GroupID,
		UserID:  arg.UserID,
	})

	return nil
}

func (q *FakeQuerier) InsertLicense(
	_ context.Context, arg database.InsertLicenseParams,
) (database.License, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.License{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	l := database.License{
		ID:         q.lastLicenseID + 1,
		UploadedAt: arg.UploadedAt,
		JWT:        arg.JWT,
		Exp:        arg.Exp,
	}
	q.lastLicenseID = l.ID
	q.licenses = append(q.licenses, l)
	return l, nil
}

func (q *FakeQuerier) InsertMissingGroups(_ context.Context, arg database.InsertMissingGroupsParams) ([]database.Group, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	groupNameMap := make(map[string]struct{})
	for _, g := range arg.GroupNames {
		groupNameMap[g] = struct{}{}
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, g := range q.groups {
		if g.OrganizationID != arg.OrganizationID {
			continue
		}
		delete(groupNameMap, g.Name)
	}

	newGroups := make([]database.Group, 0, len(groupNameMap))
	for k := range groupNameMap {
		g := database.Group{
			ID:             uuid.New(),
			Name:           k,
			OrganizationID: arg.OrganizationID,
			AvatarURL:      "",
			QuotaAllowance: 0,
			DisplayName:    "",
			Source:         arg.Source,
		}
		q.groups = append(q.groups, g)
		newGroups = append(newGroups, g)
	}

	return newGroups, nil
}

func (q *FakeQuerier) InsertOrganization(_ context.Context, arg database.InsertOrganizationParams) (database.Organization, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Organization{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	organization := database.Organization{
		ID:        arg.ID,
		Name:      arg.Name,
		CreatedAt: arg.CreatedAt,
		UpdatedAt: arg.UpdatedAt,
	}
	q.organizations = append(q.organizations, organization)
	return organization, nil
}

func (q *FakeQuerier) InsertOrganizationMember(_ context.Context, arg database.InsertOrganizationMemberParams) (database.OrganizationMember, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.OrganizationMember{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	organizationMember := database.OrganizationMember{
		OrganizationID: arg.OrganizationID,
		UserID:         arg.UserID,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		Roles:          arg.Roles,
	}
	q.organizationMembers = append(q.organizationMembers, organizationMember)
	return organizationMember, nil
}

func (q *FakeQuerier) InsertProvisionerDaemon(_ context.Context, arg database.InsertProvisionerDaemonParams) (database.ProvisionerDaemon, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.ProvisionerDaemon{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	daemon := database.ProvisionerDaemon{
		ID:           arg.ID,
		CreatedAt:    arg.CreatedAt,
		Name:         arg.Name,
		Provisioners: arg.Provisioners,
		Tags:         arg.Tags,
	}
	q.provisionerDaemons = append(q.provisionerDaemons, daemon)
	return daemon, nil
}

func (q *FakeQuerier) InsertProvisionerJob(_ context.Context, arg database.InsertProvisionerJobParams) (database.ProvisionerJob, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.ProvisionerJob{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	job := database.ProvisionerJob{
		ID:             arg.ID,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		OrganizationID: arg.OrganizationID,
		InitiatorID:    arg.InitiatorID,
		Provisioner:    arg.Provisioner,
		StorageMethod:  arg.StorageMethod,
		FileID:         arg.FileID,
		Type:           arg.Type,
		Input:          arg.Input,
		Tags:           arg.Tags,
		TraceMetadata:  arg.TraceMetadata,
	}
	job.JobStatus = provisonerJobStatus(job)
	q.provisionerJobs = append(q.provisionerJobs, job)
	return job, nil
}

func (q *FakeQuerier) InsertProvisionerJobLogs(_ context.Context, arg database.InsertProvisionerJobLogsParams) ([]database.ProvisionerJobLog, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	logs := make([]database.ProvisionerJobLog, 0)
	id := int64(1)
	if len(q.provisionerJobLogs) > 0 {
		id = q.provisionerJobLogs[len(q.provisionerJobLogs)-1].ID
	}
	for index, output := range arg.Output {
		id++
		logs = append(logs, database.ProvisionerJobLog{
			ID:        id,
			JobID:     arg.JobID,
			CreatedAt: arg.CreatedAt[index],
			Source:    arg.Source[index],
			Level:     arg.Level[index],
			Stage:     arg.Stage[index],
			Output:    output,
		})
	}
	q.provisionerJobLogs = append(q.provisionerJobLogs, logs...)
	return logs, nil
}

func (q *FakeQuerier) InsertReplica(_ context.Context, arg database.InsertReplicaParams) (database.Replica, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Replica{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	replica := database.Replica{
		ID:              arg.ID,
		CreatedAt:       arg.CreatedAt,
		StartedAt:       arg.StartedAt,
		UpdatedAt:       arg.UpdatedAt,
		Hostname:        arg.Hostname,
		RegionID:        arg.RegionID,
		RelayAddress:    arg.RelayAddress,
		Version:         arg.Version,
		DatabaseLatency: arg.DatabaseLatency,
		Primary:         arg.Primary,
	}
	q.replicas = append(q.replicas, replica)
	return replica, nil
}

func (q *FakeQuerier) InsertTemplate(_ context.Context, arg database.InsertTemplateParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	template := database.TemplateTable{
		ID:                           arg.ID,
		CreatedAt:                    arg.CreatedAt,
		UpdatedAt:                    arg.UpdatedAt,
		OrganizationID:               arg.OrganizationID,
		Name:                         arg.Name,
		Provisioner:                  arg.Provisioner,
		ActiveVersionID:              arg.ActiveVersionID,
		Description:                  arg.Description,
		CreatedBy:                    arg.CreatedBy,
		UserACL:                      arg.UserACL,
		GroupACL:                     arg.GroupACL,
		DisplayName:                  arg.DisplayName,
		Icon:                         arg.Icon,
		AllowUserCancelWorkspaceJobs: arg.AllowUserCancelWorkspaceJobs,
		AllowUserAutostart:           true,
		AllowUserAutostop:            true,
	}
	q.templates = append(q.templates, template)
	return nil
}

func (q *FakeQuerier) InsertTemplateVersion(_ context.Context, arg database.InsertTemplateVersionParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	if len(arg.Message) > 1048576 {
		return xerrors.New("message too long")
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	version := database.TemplateVersionTable{
		ID:             arg.ID,
		TemplateID:     arg.TemplateID,
		OrganizationID: arg.OrganizationID,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		Name:           arg.Name,
		Message:        arg.Message,
		Readme:         arg.Readme,
		JobID:          arg.JobID,
		CreatedBy:      arg.CreatedBy,
	}
	q.templateVersions = append(q.templateVersions, version)
	return nil
}

func (q *FakeQuerier) InsertTemplateVersionParameter(_ context.Context, arg database.InsertTemplateVersionParameterParams) (database.TemplateVersionParameter, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.TemplateVersionParameter{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	param := database.TemplateVersionParameter{
		TemplateVersionID:   arg.TemplateVersionID,
		Name:                arg.Name,
		DisplayName:         arg.DisplayName,
		Description:         arg.Description,
		Type:                arg.Type,
		Mutable:             arg.Mutable,
		DefaultValue:        arg.DefaultValue,
		Icon:                arg.Icon,
		Options:             arg.Options,
		ValidationError:     arg.ValidationError,
		ValidationRegex:     arg.ValidationRegex,
		ValidationMin:       arg.ValidationMin,
		ValidationMax:       arg.ValidationMax,
		ValidationMonotonic: arg.ValidationMonotonic,
		Required:            arg.Required,
		DisplayOrder:        arg.DisplayOrder,
		Ephemeral:           arg.Ephemeral,
	}
	q.templateVersionParameters = append(q.templateVersionParameters, param)
	return param, nil
}

func (q *FakeQuerier) InsertTemplateVersionVariable(_ context.Context, arg database.InsertTemplateVersionVariableParams) (database.TemplateVersionVariable, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.TemplateVersionVariable{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	variable := database.TemplateVersionVariable{
		TemplateVersionID: arg.TemplateVersionID,
		Name:              arg.Name,
		Description:       arg.Description,
		Type:              arg.Type,
		Value:             arg.Value,
		DefaultValue:      arg.DefaultValue,
		Required:          arg.Required,
		Sensitive:         arg.Sensitive,
	}
	q.templateVersionVariables = append(q.templateVersionVariables, variable)
	return variable, nil
}

func (q *FakeQuerier) InsertUser(_ context.Context, arg database.InsertUserParams) (database.User, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.User{}, err
	}

	// There is a common bug when using dbfake that 2 inserted users have the
	// same created_at time. This causes user order to not be deterministic,
	// which breaks some unit tests.
	// To fix this, we make sure that the created_at time is always greater
	// than the last user's created_at time.
	allUsers, _ := q.GetUsers(context.Background(), database.GetUsersParams{})
	if len(allUsers) > 0 {
		lastUser := allUsers[len(allUsers)-1]
		if arg.CreatedAt.Before(lastUser.CreatedAt) ||
			arg.CreatedAt.Equal(lastUser.CreatedAt) {
			// 1 ms is a good enough buffer.
			arg.CreatedAt = lastUser.CreatedAt.Add(time.Millisecond)
		}
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, user := range q.users {
		if user.Username == arg.Username && !user.Deleted {
			return database.User{}, errDuplicateKey
		}
	}

	user := database.User{
		ID:             arg.ID,
		Email:          arg.Email,
		HashedPassword: arg.HashedPassword,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		Username:       arg.Username,
		Status:         database.UserStatusDormant,
		RBACRoles:      arg.RBACRoles,
		LoginType:      arg.LoginType,
	}
	q.users = append(q.users, user)
	return user, nil
}

func (q *FakeQuerier) InsertUserGroupsByName(_ context.Context, arg database.InsertUserGroupsByNameParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	var groupIDs []uuid.UUID
	for _, group := range q.groups {
		for _, groupName := range arg.GroupNames {
			if group.Name == groupName {
				groupIDs = append(groupIDs, group.ID)
			}
		}
	}

	for _, groupID := range groupIDs {
		q.groupMembers = append(q.groupMembers, database.GroupMember{
			UserID:  arg.UserID,
			GroupID: groupID,
		})
	}

	return nil
}

func (q *FakeQuerier) InsertUserLink(_ context.Context, args database.InsertUserLinkParams) (database.UserLink, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	link := database.UserLink{
		UserID:                 args.UserID,
		LoginType:              args.LoginType,
		LinkedID:               args.LinkedID,
		OAuthAccessToken:       args.OAuthAccessToken,
		OAuthAccessTokenKeyID:  args.OAuthAccessTokenKeyID,
		OAuthRefreshToken:      args.OAuthRefreshToken,
		OAuthRefreshTokenKeyID: args.OAuthRefreshTokenKeyID,
		OAuthExpiry:            args.OAuthExpiry,
	}

	q.userLinks = append(q.userLinks, link)

	return link, nil
}

func (q *FakeQuerier) InsertWorkspace(_ context.Context, arg database.InsertWorkspaceParams) (database.Workspace, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Workspace{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	workspace := database.Workspace{
		ID:                arg.ID,
		CreatedAt:         arg.CreatedAt,
		UpdatedAt:         arg.UpdatedAt,
		OwnerID:           arg.OwnerID,
		OrganizationID:    arg.OrganizationID,
		TemplateID:        arg.TemplateID,
		Name:              arg.Name,
		AutostartSchedule: arg.AutostartSchedule,
		Ttl:               arg.Ttl,
		LastUsedAt:        arg.LastUsedAt,
		AutomaticUpdates:  arg.AutomaticUpdates,
	}
	q.workspaces = append(q.workspaces, workspace)
	return workspace, nil
}

func (q *FakeQuerier) InsertWorkspaceAgent(_ context.Context, arg database.InsertWorkspaceAgentParams) (database.WorkspaceAgent, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.WorkspaceAgent{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	agent := database.WorkspaceAgent{
		ID:                       arg.ID,
		CreatedAt:                arg.CreatedAt,
		UpdatedAt:                arg.UpdatedAt,
		ResourceID:               arg.ResourceID,
		AuthToken:                arg.AuthToken,
		AuthInstanceID:           arg.AuthInstanceID,
		EnvironmentVariables:     arg.EnvironmentVariables,
		Name:                     arg.Name,
		Architecture:             arg.Architecture,
		OperatingSystem:          arg.OperatingSystem,
		Directory:                arg.Directory,
		InstanceMetadata:         arg.InstanceMetadata,
		ResourceMetadata:         arg.ResourceMetadata,
		ConnectionTimeoutSeconds: arg.ConnectionTimeoutSeconds,
		TroubleshootingURL:       arg.TroubleshootingURL,
		MOTDFile:                 arg.MOTDFile,
		LifecycleState:           database.WorkspaceAgentLifecycleStateCreated,
		DisplayApps:              arg.DisplayApps,
	}

	q.workspaceAgents = append(q.workspaceAgents, agent)
	return agent, nil
}

func (q *FakeQuerier) InsertWorkspaceAgentLogSources(_ context.Context, arg database.InsertWorkspaceAgentLogSourcesParams) ([]database.WorkspaceAgentLogSource, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	logSources := make([]database.WorkspaceAgentLogSource, 0)
	for index, source := range arg.ID {
		logSource := database.WorkspaceAgentLogSource{
			ID:               source,
			WorkspaceAgentID: arg.WorkspaceAgentID,
			CreatedAt:        arg.CreatedAt,
			DisplayName:      arg.DisplayName[index],
			Icon:             arg.Icon[index],
		}
		logSources = append(logSources, logSource)
	}
	q.workspaceAgentLogSources = append(q.workspaceAgentLogSources, logSources...)
	return logSources, nil
}

func (q *FakeQuerier) InsertWorkspaceAgentLogs(_ context.Context, arg database.InsertWorkspaceAgentLogsParams) ([]database.WorkspaceAgentLog, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	logs := []database.WorkspaceAgentLog{}
	id := int64(0)
	if len(q.workspaceAgentLogs) > 0 {
		id = q.workspaceAgentLogs[len(q.workspaceAgentLogs)-1].ID
	}
	outputLength := int32(0)
	for index, output := range arg.Output {
		id++
		logs = append(logs, database.WorkspaceAgentLog{
			ID:          id,
			AgentID:     arg.AgentID,
			CreatedAt:   arg.CreatedAt,
			Level:       arg.Level[index],
			LogSourceID: arg.LogSourceID,
			Output:      output,
		})
		outputLength += int32(len(output))
	}
	for index, agent := range q.workspaceAgents {
		if agent.ID != arg.AgentID {
			continue
		}
		// Greater than 1MB, same as the PostgreSQL constraint!
		if agent.LogsLength+outputLength > (1 << 20) {
			return nil, &pq.Error{
				Constraint: "max_logs_length",
				Table:      "workspace_agents",
			}
		}
		agent.LogsLength += outputLength
		q.workspaceAgents[index] = agent
		break
	}
	q.workspaceAgentLogs = append(q.workspaceAgentLogs, logs...)
	return logs, nil
}

func (q *FakeQuerier) InsertWorkspaceAgentMetadata(_ context.Context, arg database.InsertWorkspaceAgentMetadataParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	metadatum := database.WorkspaceAgentMetadatum{
		WorkspaceAgentID: arg.WorkspaceAgentID,
		Script:           arg.Script,
		DisplayName:      arg.DisplayName,
		Key:              arg.Key,
		Timeout:          arg.Timeout,
		Interval:         arg.Interval,
	}

	q.workspaceAgentMetadata = append(q.workspaceAgentMetadata, metadatum)
	return nil
}

func (q *FakeQuerier) InsertWorkspaceAgentScripts(_ context.Context, arg database.InsertWorkspaceAgentScriptsParams) ([]database.WorkspaceAgentScript, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	scripts := make([]database.WorkspaceAgentScript, 0)
	for index, source := range arg.LogSourceID {
		script := database.WorkspaceAgentScript{
			LogSourceID:      source,
			WorkspaceAgentID: arg.WorkspaceAgentID,
			LogPath:          arg.LogPath[index],
			Script:           arg.Script[index],
			Cron:             arg.Cron[index],
			StartBlocksLogin: arg.StartBlocksLogin[index],
			RunOnStart:       arg.RunOnStart[index],
			RunOnStop:        arg.RunOnStop[index],
			TimeoutSeconds:   arg.TimeoutSeconds[index],
			CreatedAt:        arg.CreatedAt,
		}
		scripts = append(scripts, script)
	}
	q.workspaceAgentScripts = append(q.workspaceAgentScripts, scripts...)
	return scripts, nil
}

func (q *FakeQuerier) InsertWorkspaceAgentStat(_ context.Context, p database.InsertWorkspaceAgentStatParams) (database.WorkspaceAgentStat, error) {
	if err := validateDatabaseType(p); err != nil {
		return database.WorkspaceAgentStat{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	stat := database.WorkspaceAgentStat{
		ID:                          p.ID,
		CreatedAt:                   p.CreatedAt,
		WorkspaceID:                 p.WorkspaceID,
		AgentID:                     p.AgentID,
		UserID:                      p.UserID,
		ConnectionsByProto:          p.ConnectionsByProto,
		ConnectionCount:             p.ConnectionCount,
		RxPackets:                   p.RxPackets,
		RxBytes:                     p.RxBytes,
		TxPackets:                   p.TxPackets,
		TxBytes:                     p.TxBytes,
		TemplateID:                  p.TemplateID,
		SessionCountVSCode:          p.SessionCountVSCode,
		SessionCountJetBrains:       p.SessionCountJetBrains,
		SessionCountReconnectingPTY: p.SessionCountReconnectingPTY,
		SessionCountSSH:             p.SessionCountSSH,
		ConnectionMedianLatencyMS:   p.ConnectionMedianLatencyMS,
	}
	q.workspaceAgentStats = append(q.workspaceAgentStats, stat)
	return stat, nil
}

func (q *FakeQuerier) InsertWorkspaceAgentStats(_ context.Context, arg database.InsertWorkspaceAgentStatsParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	var connectionsByProto []map[string]int64
	if err := json.Unmarshal(arg.ConnectionsByProto, &connectionsByProto); err != nil {
		return err
	}
	for i := 0; i < len(arg.ID); i++ {
		cbp, err := json.Marshal(connectionsByProto[i])
		if err != nil {
			return xerrors.Errorf("failed to marshal connections_by_proto: %w", err)
		}
		stat := database.WorkspaceAgentStat{
			ID:                          arg.ID[i],
			CreatedAt:                   arg.CreatedAt[i],
			WorkspaceID:                 arg.WorkspaceID[i],
			AgentID:                     arg.AgentID[i],
			UserID:                      arg.UserID[i],
			ConnectionsByProto:          cbp,
			ConnectionCount:             arg.ConnectionCount[i],
			RxPackets:                   arg.RxPackets[i],
			RxBytes:                     arg.RxBytes[i],
			TxPackets:                   arg.TxPackets[i],
			TxBytes:                     arg.TxBytes[i],
			TemplateID:                  arg.TemplateID[i],
			SessionCountVSCode:          arg.SessionCountVSCode[i],
			SessionCountJetBrains:       arg.SessionCountJetBrains[i],
			SessionCountReconnectingPTY: arg.SessionCountReconnectingPTY[i],
			SessionCountSSH:             arg.SessionCountSSH[i],
			ConnectionMedianLatencyMS:   arg.ConnectionMedianLatencyMS[i],
		}
		q.workspaceAgentStats = append(q.workspaceAgentStats, stat)
	}

	return nil
}

func (q *FakeQuerier) InsertWorkspaceApp(_ context.Context, arg database.InsertWorkspaceAppParams) (database.WorkspaceApp, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.WorkspaceApp{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	if arg.SharingLevel == "" {
		arg.SharingLevel = database.AppSharingLevelOwner
	}

	// nolint:gosimple
	workspaceApp := database.WorkspaceApp{
		ID:                   arg.ID,
		AgentID:              arg.AgentID,
		CreatedAt:            arg.CreatedAt,
		Slug:                 arg.Slug,
		DisplayName:          arg.DisplayName,
		Icon:                 arg.Icon,
		Command:              arg.Command,
		Url:                  arg.Url,
		External:             arg.External,
		Subdomain:            arg.Subdomain,
		SharingLevel:         arg.SharingLevel,
		HealthcheckUrl:       arg.HealthcheckUrl,
		HealthcheckInterval:  arg.HealthcheckInterval,
		HealthcheckThreshold: arg.HealthcheckThreshold,
		Health:               arg.Health,
	}
	q.workspaceApps = append(q.workspaceApps, workspaceApp)
	return workspaceApp, nil
}

func (q *FakeQuerier) InsertWorkspaceAppStats(_ context.Context, arg database.InsertWorkspaceAppStatsParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

InsertWorkspaceAppStatsLoop:
	for i := 0; i < len(arg.UserID); i++ {
		stat := database.WorkspaceAppStat{
			ID:               q.workspaceAppStatsLastInsertID + 1,
			UserID:           arg.UserID[i],
			WorkspaceID:      arg.WorkspaceID[i],
			AgentID:          arg.AgentID[i],
			AccessMethod:     arg.AccessMethod[i],
			SlugOrPort:       arg.SlugOrPort[i],
			SessionID:        arg.SessionID[i],
			SessionStartedAt: arg.SessionStartedAt[i],
			SessionEndedAt:   arg.SessionEndedAt[i],
			Requests:         arg.Requests[i],
		}
		for j, s := range q.workspaceAppStats {
			// Check unique constraint for upsert.
			if s.UserID == stat.UserID && s.AgentID == stat.AgentID && s.SessionID == stat.SessionID {
				q.workspaceAppStats[j].SessionEndedAt = stat.SessionEndedAt
				q.workspaceAppStats[j].Requests = stat.Requests
				continue InsertWorkspaceAppStatsLoop
			}
		}
		q.workspaceAppStats = append(q.workspaceAppStats, stat)
		q.workspaceAppStatsLastInsertID++
	}

	return nil
}

func (q *FakeQuerier) InsertWorkspaceBuild(_ context.Context, arg database.InsertWorkspaceBuildParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	workspaceBuild := database.WorkspaceBuildTable{
		ID:                arg.ID,
		CreatedAt:         arg.CreatedAt,
		UpdatedAt:         arg.UpdatedAt,
		WorkspaceID:       arg.WorkspaceID,
		TemplateVersionID: arg.TemplateVersionID,
		BuildNumber:       arg.BuildNumber,
		Transition:        arg.Transition,
		InitiatorID:       arg.InitiatorID,
		JobID:             arg.JobID,
		ProvisionerState:  arg.ProvisionerState,
		Deadline:          arg.Deadline,
		MaxDeadline:       arg.MaxDeadline,
		Reason:            arg.Reason,
	}
	q.workspaceBuilds = append(q.workspaceBuilds, workspaceBuild)
	return nil
}

func (q *FakeQuerier) InsertWorkspaceBuildParameters(_ context.Context, arg database.InsertWorkspaceBuildParametersParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, name := range arg.Name {
		q.workspaceBuildParameters = append(q.workspaceBuildParameters, database.WorkspaceBuildParameter{
			WorkspaceBuildID: arg.WorkspaceBuildID,
			Name:             name,
			Value:            arg.Value[index],
		})
	}
	return nil
}

func (q *FakeQuerier) InsertWorkspaceProxy(_ context.Context, arg database.InsertWorkspaceProxyParams) (database.WorkspaceProxy, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	lastRegionID := int32(0)
	for _, p := range q.workspaceProxies {
		if !p.Deleted && p.Name == arg.Name {
			return database.WorkspaceProxy{}, errDuplicateKey
		}
		if p.RegionID > lastRegionID {
			lastRegionID = p.RegionID
		}
	}

	p := database.WorkspaceProxy{
		ID:                arg.ID,
		Name:              arg.Name,
		DisplayName:       arg.DisplayName,
		Icon:              arg.Icon,
		DerpEnabled:       arg.DerpEnabled,
		DerpOnly:          arg.DerpOnly,
		TokenHashedSecret: arg.TokenHashedSecret,
		RegionID:          lastRegionID + 1,
		CreatedAt:         arg.CreatedAt,
		UpdatedAt:         arg.UpdatedAt,
		Deleted:           false,
	}
	q.workspaceProxies = append(q.workspaceProxies, p)
	return p, nil
}

func (q *FakeQuerier) InsertWorkspaceResource(_ context.Context, arg database.InsertWorkspaceResourceParams) (database.WorkspaceResource, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.WorkspaceResource{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	resource := database.WorkspaceResource{
		ID:         arg.ID,
		CreatedAt:  arg.CreatedAt,
		JobID:      arg.JobID,
		Transition: arg.Transition,
		Type:       arg.Type,
		Name:       arg.Name,
		Hide:       arg.Hide,
		Icon:       arg.Icon,
		DailyCost:  arg.DailyCost,
	}
	q.workspaceResources = append(q.workspaceResources, resource)
	return resource, nil
}

func (q *FakeQuerier) InsertWorkspaceResourceMetadata(_ context.Context, arg database.InsertWorkspaceResourceMetadataParams) ([]database.WorkspaceResourceMetadatum, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	metadata := make([]database.WorkspaceResourceMetadatum, 0)
	id := int64(1)
	if len(q.workspaceResourceMetadata) > 0 {
		id = q.workspaceResourceMetadata[len(q.workspaceResourceMetadata)-1].ID
	}
	for index, key := range arg.Key {
		id++
		value := arg.Value[index]
		metadata = append(metadata, database.WorkspaceResourceMetadatum{
			ID:                  id,
			WorkspaceResourceID: arg.WorkspaceResourceID,
			Key:                 key,
			Value: sql.NullString{
				String: value,
				Valid:  value != "",
			},
		})
		payload, _ := json.Marshal(provisionerdserver.TemplateVersionImportJob{
			TemplateVersionID: templateVersion.ID,
		})
		dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			ID:             jobID,
			OrganizationID: ws.OrganizationID,
			Input:          payload,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			CompletedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
		})
		ProvisionerJobResources(t, db, jobID, seed.Transition, resources...)
		seed.TemplateVersionID = templateVersion.ID
	}
	build := dbgen.WorkspaceBuild(t, db, seed)
	ProvisionerJobResources(t, db, job.ID, seed.Transition, resources...)
	return build
}

// ProvisionerJobResources inserts a series of resources into a provisioner job.
func ProvisionerJobResources(t testing.TB, db database.Store, job uuid.UUID, transition database.WorkspaceTransition, resources ...*sdkproto.Resource) {
	t.Helper()
	if transition == "" {
		// Default to start!
		transition = database.WorkspaceTransitionStart
	}
	for _, resource := range resources {
		//nolint:gocritic // This is only used by tests.
		err := provisionerdserver.InsertWorkspaceResource(dbauthz.AsSystemRestricted(context.Background()), db, job, transition, resource, &telemetry.Snapshot{})
		require.NoError(t, err)
	}
}
