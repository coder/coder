package idpsync

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// The rewriter is the beginning of dealing with legacy settings: settings
// that are not valid, whether saved before the current checks existed or
// gone stale since. It runs when settings are saved or fetched, and brings
// them into valid form where a rewrite rule allows, so that what is stored
// and returned becomes valid over time.
//
// Its rules R6 and R7 remove UUIDs that name no group or a group in another
// organization. partitionOutInvalidSettings, which SyncGroups calls, drops
// the same UUIDs at sync time. The duplication is intentional, because the
// two serve different purposes. The partition guards sync against decay in
// validity: settings that were valid can become invalid between one fetch and
// the next login, when other data changes, and the partition is what keeps
// sync from acting on them. It must remain unaltered. The rewriter repairs
// settings it is given, and makes no promise about what happens to them
// afterward.

// GroupSyncRewriteOutcome is what the rewriter did with the settings as a
// whole.
type GroupSyncRewriteOutcome string

const (
	// GroupSyncRewriteOutcomeValid: the settings had no findings and were
	// returned unchanged, whether or not they were in canonical form.
	GroupSyncRewriteOutcomeValid GroupSyncRewriteOutcome = "valid"
	// GroupSyncRewriteOutcomeRewritten: the settings were brought into valid
	// canonical form.
	GroupSyncRewriteOutcomeRewritten GroupSyncRewriteOutcome = "rewritten"
	// GroupSyncRewriteOutcomeNotRewritten: the settings were returned
	// unchanged.
	GroupSyncRewriteOutcomeNotRewritten GroupSyncRewriteOutcome = "not_rewritten"
)

// GroupSyncRewriteResult is the result of carrying out a rewrite plan.
type GroupSyncRewriteResult struct {
	// Outcome is what the rewriter did with the settings as a whole.
	Outcome GroupSyncRewriteOutcome
	// Settings is the settings to save or return: in canonical form when the
	// outcome is rewritten, and the input unchanged otherwise.
	Settings codersdk.GroupSyncSettings
	// Warnings holds one warning for each finding, in the same order as the
	// findings.
	Warnings []codersdk.GroupSyncWarning
}

// RewriteResult carries out plan on settings. The plan must be the one
// RewritePlan gave for the settings' findings; it carries each finding with
// its action. The rewriter makes no decisions of its own:
//
//   - accept: settings are returned unchanged, with no warnings;
//   - rewrite: every finding's rewrite rule is applied and the result is
//     returned in canonical form, with one warning per finding;
//   - keep: settings are returned unchanged, with one warning per finding;
//   - reject: RewriteResult returns an error, since there is nothing to
//     carry out.
func RewriteResult(settings codersdk.GroupSyncSettings, plan GroupSyncRewritePlan) (GroupSyncRewriteResult, error) {
	findings := make([]codersdk.GroupSyncFinding, 0, len(plan.Actions))
	for _, planned := range plan.Actions {
		findings = append(findings, planned.Finding)
	}

	switch plan.Instruction {
	case GroupSyncInstructionAccept:
		return GroupSyncRewriteResult{
			Outcome:  GroupSyncRewriteOutcomeValid,
			Settings: settings,
			Warnings: []codersdk.GroupSyncWarning{},
		}, nil

	case GroupSyncInstructionKeep:
		warnings := make([]codersdk.GroupSyncWarning, 0, len(findings))
		for _, f := range findings {
			received, err := elementJSON(settings, f.Path)
			if err != nil {
				return GroupSyncRewriteResult{}, err
			}
			warnings = append(warnings, codersdk.GroupSyncWarning{Code: f.Code, Path: f.Path, Received: received, Accepted: received})
		}
		return GroupSyncRewriteResult{
			Outcome:  GroupSyncRewriteOutcomeNotRewritten,
			Settings: settings,
			Warnings: warnings,
		}, nil

	case GroupSyncInstructionRewrite:
		return rewriteSettings(settings, findings)

	case GroupSyncInstructionReject:
		return GroupSyncRewriteResult{}, xerrors.New("plan rejects the settings, so there is nothing to carry out")

	default:
		return GroupSyncRewriteResult{}, xerrors.Errorf("plan has unknown instruction %q", plan.Instruction)
	}
}

// rewriteSettings applies the rewrite rule of every finding to settings and
// returns the result in canonical form. Every finding must be rewritable.
func rewriteSettings(settings codersdk.GroupSyncSettings, findings []codersdk.GroupSyncFinding) (GroupSyncRewriteResult, error) {
	// Each rule removes a member, removes an element, or replaces a `null`
	// array with an empty one. Collect them first, so that the rules apply to
	// the settings as received and paths keep their meaning.
	removedMembers := make(map[string]bool)
	removedElements := make(map[string]map[int]bool)
	warnings := make([]codersdk.GroupSyncWarning, 0, len(findings))

	for _, f := range findings {
		received, err := elementJSON(settings, f.Path)
		if err != nil {
			return GroupSyncRewriteResult{}, err
		}
		key, index, hasIndex, ok := parseMappingPath(f.Path)
		warning := codersdk.GroupSyncWarning{Code: f.Code, Path: f.Path, Received: received}

		switch f.Code {
		case codersdk.GroupSyncFindingNilUUID, // R1
			codersdk.GroupSyncFindingRepeatedUUID,  // R2
			codersdk.GroupSyncFindingEveryoneGroup, // R5
			codersdk.GroupSyncFindingMissingGroup,  // R6
			codersdk.GroupSyncFindingOutsideGroup:  // R7
			if !ok || !hasIndex {
				return GroupSyncRewriteResult{}, xerrors.Errorf("finding %q has path %q, which is not a mapping element", f.Code, f.Path)
			}
			if removedElements[key] == nil {
				removedElements[key] = make(map[int]bool)
			}
			removedElements[key][index] = true

		case codersdk.GroupSyncFindingNullArray: // R3
			if !ok || hasIndex {
				return GroupSyncRewriteResult{}, xerrors.Errorf("finding %q has path %q, which is not a mapping member", f.Code, f.Path)
			}
			// The member is kept, so its replacement is accepted.
			warning.Accepted = json.RawMessage(`[]`)

		case codersdk.GroupSyncFindingFilterExcludedKey: // R4
			if !ok || hasIndex {
				return GroupSyncRewriteResult{}, xerrors.Errorf("finding %q has path %q, which is not a mapping member", f.Code, f.Path)
			}
			removedMembers[key] = true

		default:
			return GroupSyncRewriteResult{}, xerrors.Errorf("finding code %q has no rewrite rule", f.Code)
		}
		warnings = append(warnings, warning)
	}

	// Canonical form: `mapping` is an object, never `null`; an empty
	// `regex_filter` is `null`; an empty legacy mapping is absent. A rewrite
	// only happens with no legacy finding, so the legacy mapping is empty.
	rewritten := settings
	rewritten.Mapping = make(map[string][]uuid.UUID, len(settings.Mapping))
	for key, ids := range settings.Mapping {
		if removedMembers[key] {
			continue
		}
		// An array emptied by removals, or `null` in the input, becomes an
		// empty array. The member stays, since it keeps its name out of N.
		kept := make([]uuid.UUID, 0, len(ids))
		for i, id := range ids {
			if !removedElements[key][i] {
				kept = append(kept, id)
			}
		}
		rewritten.Mapping[key] = kept
	}
	if rewritten.RegexFilter != nil && rewritten.RegexFilter.String() == "" {
		rewritten.RegexFilter = nil
	}
	rewritten.LegacyNameMapping = nil

	return GroupSyncRewriteResult{
		Outcome:  GroupSyncRewriteOutcomeRewritten,
		Settings: rewritten,
		Warnings: warnings,
	}, nil
}

// parseMappingPath recovers the `mapping` key, and the array index when there
// is one, from a finding's path. It reports ok as false for a path that is
// not under `mapping`.
func parseMappingPath(path string) (key string, index int, hasIndex bool, ok bool) {
	tokens := strings.Split(path, "/")
	// A path under `mapping` is "", "mapping", key, and maybe an index.
	if len(tokens) < 3 || len(tokens) > 4 || tokens[0] != "" || tokens[1] != "mapping" {
		return "", 0, false, false
	}
	key = codersdk.UnescapeJSONPointerToken(tokens[2])
	if len(tokens) == 4 {
		i, err := strconv.Atoi(tokens[3])
		if err != nil || i < 0 {
			return "", 0, false, false
		}
		return key, i, true, true
	}
	return key, 0, false, true
}

// elementJSON returns the JSON text of the element of settings that a
// finding's path refers to. The empty path refers to the whole settings.
func elementJSON(settings codersdk.GroupSyncSettings, path string) (json.RawMessage, error) {
	var element any
	switch path {
	case "":
		element = settings
	case "/field":
		element = settings.Field
	case "/legacy_group_name_mapping":
		element = settings.LegacyNameMapping
	default:
		key, index, hasIndex, ok := parseMappingPath(path)
		if !ok {
			return nil, xerrors.Errorf("finding path %q refers to no element of the settings", path)
		}
		ids, present := settings.Mapping[key]
		switch {
		case !present:
			return nil, xerrors.Errorf("finding path %q names a mapping key that is not present", path)
		case !hasIndex:
			element = ids
		case index >= len(ids):
			return nil, xerrors.Errorf("finding path %q indexes past the end of its array", path)
		default:
			element = ids[index]
		}
	}
	b, err := json.Marshal(element)
	if err != nil {
		return nil, xerrors.Errorf("encode element at %q: %w", path, err)
	}
	return b, nil
}
