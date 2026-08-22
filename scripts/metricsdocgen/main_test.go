package main

import (
	"bytes"
	"strconv"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

const testDocPath = "docs/ai-coder/ai-gateway/monitoring.md"

func TestParseSection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		value   string
		want    documentSection
		wantErr string
	}{
		{
			name:  "prefix",
			value: "common=prefix:coder_ai_gateway_,!coder_ai_gateway_proxy_",
			want: documentSection{
				name: "common",
				filter: metricFilter{
					prefixes:        []string{"coder_ai_gateway_"},
					excludePrefixes: []string{"coder_ai_gateway_proxy_"},
				},
			},
		},
		{
			name:  "multiple prefixes and exclusions",
			value: "common=prefix:coder_ai_gateway_,coder_other_,!coder_ai_gateway_proxy_,!coder_ai_gateway_cost_control_",
			want: documentSection{
				name: "common",
				filter: metricFilter{
					prefixes: []string{"coder_ai_gateway_", "coder_other_"},
					excludePrefixes: []string{
						"coder_ai_gateway_proxy_",
						"coder_ai_gateway_cost_control_",
					},
				},
			},
		},
		{
			name:    "missing filter",
			value:   "common",
			wantErr: "section must have the form",
		},
		{
			name:    "invalid name",
			value:   "bad name=prefix:metric_",
			wantErr: "may contain only",
		},
		{
			name:    "invalid kind",
			value:   "common=contains:metric",
			wantErr: "unknown filter kind",
		},
		{
			name:    "missing filter kind",
			value:   "common=coder_ai_gateway_",
			wantErr: "must have the form prefix:VALUE",
		},
		{
			name:    "empty filter value",
			value:   "common=prefix:coder_ai_gateway_,,coder_other_",
			wantErr: "empty filter value",
		},
		{
			name:    "empty excluded prefix",
			value:   "common=prefix:coder_ai_gateway_,!",
			wantErr: "empty excluded prefix",
		},
		{
			name:    "only exclusions",
			value:   "common=prefix:!coder_ai_gateway_proxy_",
			wantErr: "requires at least one included prefix",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseSection(testCase.value)
			if testCase.wantErr != "" {
				require.ErrorContains(t, err, testCase.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.want, got)
		})
	}
}

func TestSectionFlagsSetRejectsDuplicates(t *testing.T) {
	t.Parallel()

	var flags sectionFlags
	require.NoError(t, flags.Set("common=prefix:coder_ai_gateway_"))
	require.ErrorContains(t, flags.Set("common=prefix:coder_other_"), "duplicate section")
	require.NoError(t, flags.Set("proxy=prefix:coder_ai_gateway_proxy_"))
	require.Equal(t, "common,proxy", flags.String())
}

func TestFilterMetricFamilies(t *testing.T) {
	t.Parallel()

	metrics := []*dto.MetricFamily{
		metricFamily("coder_ai_gateway_cost_control_blocked_users", dto.MetricType_GAUGE, "Blocked users.", "group_id"),
		metricFamily("coder_ai_gateway_interceptions_total", dto.MetricType_COUNTER, "Intercepted requests.", "provider"),
		metricFamily("coder_ai_gateway_proxy_mitm_requests_total", dto.MetricType_COUNTER, "Proxy requests.", "provider"),
	}

	byPrefix := filterMetricFamilies(metrics, metricFilter{prefixes: []string{"coder_ai_gateway_proxy_"}})
	require.Equal(t, []string{"coder_ai_gateway_proxy_mitm_requests_total"}, metricNames(byPrefix))

	byExcludedPrefix := filterMetricFamilies(metrics, metricFilter{
		prefixes: []string{"coder_ai_gateway_"},
		excludePrefixes: []string{
			"coder_ai_gateway_cost_control_",
			"coder_ai_gateway_proxy_",
		},
	})
	require.Equal(t, []string{"coder_ai_gateway_interceptions_total"}, metricNames(byExcludedPrefix))

	require.Empty(t, filterMetricFamilies(metrics, metricFilter{prefixes: []string{"coder_absent_"}}))
}

func TestUpdateDocumentSections(t *testing.T) {
	t.Parallel()

	commonPrefix := string(namedGeneratorPrefix(testDocPath, "common"))
	commonSuffix := string(namedGeneratorSuffix(testDocPath, "common"))
	proxyPrefix := string(namedGeneratorPrefix(testDocPath, "proxy"))
	proxySuffix := string(namedGeneratorSuffix(testDocPath, "proxy"))
	doc := []byte("before\n" + commonPrefix + "\nstale common\n" + commonSuffix + "\nmiddle\n" + proxyPrefix + "\nstale proxy\n" + proxySuffix + "\nafter\n")
	metrics := []*dto.MetricFamily{
		metricFamily("coder_ai_gateway_interceptions_total", dto.MetricType_COUNTER, "Intercepted requests.", "provider", "model"),
		metricFamily("coder_ai_gateway_proxy_mitm_requests_total", dto.MetricType_COUNTER, "Proxy requests.", "provider"),
	}
	sections := []documentSection{
		{name: "common", filter: metricFilter{
			prefixes:        []string{"coder_ai_gateway_"},
			excludePrefixes: []string{"coder_ai_gateway_proxy_"},
		}},
		{name: "proxy", filter: metricFilter{prefixes: []string{"coder_ai_gateway_proxy_"}}},
	}

	updated, err := updateDocumentSections(doc, metrics, testDocPath, sections)
	require.NoError(t, err)
	require.NotContains(t, string(updated), "stale common")
	require.NotContains(t, string(updated), "stale proxy")

	commonBody := sectionBody(t, updated, commonPrefix, commonSuffix)
	require.Contains(t, commonBody, "| `coder_ai_gateway_interceptions_total` | counter | Intercepted requests. | `model` `provider` |")
	require.NotContains(t, commonBody, "coder_ai_gateway_proxy_mitm_requests_total")

	proxyBody := sectionBody(t, updated, proxyPrefix, proxySuffix)
	require.Contains(t, proxyBody, "| `coder_ai_gateway_proxy_mitm_requests_total` | counter | Proxy requests. | `provider` |")
	require.NotContains(t, proxyBody, "coder_ai_gateway_interceptions_total`")

	updatedAgain, err := updateDocumentSections(updated, metrics, testDocPath, sections)
	require.NoError(t, err)
	require.Equal(t, updated, updatedAgain)
}

func TestUpdateDocumentSectionsRejectsEmptySection(t *testing.T) {
	t.Parallel()

	prefix := string(namedGeneratorPrefix(testDocPath, "bogus"))
	suffix := string(namedGeneratorSuffix(testDocPath, "bogus"))
	doc := []byte(prefix + "\nstale\n" + suffix + "\n")
	metrics := []*dto.MetricFamily{
		metricFamily("coder_ai_gateway_interceptions_total", dto.MetricType_COUNTER, "Intercepted requests.", "provider"),
	}
	sections := []documentSection{
		{name: "bogus", filter: metricFilter{prefixes: []string{"coder_absent_"}}},
	}

	_, err := updateDocumentSections(doc, metrics, testDocPath, sections)
	require.ErrorContains(t, err, `section "bogus" matches no metrics`)
}

func TestUpdateDocumentSectionsRejectsOrphanSection(t *testing.T) {
	t.Parallel()

	commonPrefix := string(namedGeneratorPrefix(testDocPath, "common"))
	commonSuffix := string(namedGeneratorSuffix(testDocPath, "common"))
	orphanPrefix := string(namedGeneratorPrefix(testDocPath, "circuit-breakers"))
	orphanSuffix := string(namedGeneratorSuffix(testDocPath, "circuit-breakers"))
	doc := []byte(commonPrefix + "\nstale common\n" + commonSuffix + "\n" + orphanPrefix + "\nstale orphan\n" + orphanSuffix + "\n")
	metrics := []*dto.MetricFamily{
		metricFamily("coder_ai_gateway_interceptions_total", dto.MetricType_COUNTER, "Intercepted requests.", "provider"),
	}
	sections := []documentSection{
		{name: "common", filter: metricFilter{prefixes: []string{"coder_ai_gateway_"}}},
	}

	_, err := updateDocumentSections(doc, metrics, testDocPath, sections)
	require.ErrorContains(t, err, `no --section flag claims: "circuit-breakers"`)
}

// A closing marker with no opening one reads as a section boundary that no
// invocation owns.
func TestUpdateDocumentSectionsRejectsStrayClosingMarker(t *testing.T) {
	t.Parallel()

	prefix := string(namedGeneratorPrefix(testDocPath, "common"))
	suffix := string(namedGeneratorSuffix(testDocPath, "common"))
	stray := string(namedGeneratorSuffix(testDocPath, "circuit-breakers"))
	doc := []byte(prefix + "\nstale\n" + suffix + "\n" + stray + "\n")
	metrics := []*dto.MetricFamily{
		metricFamily("coder_ai_gateway_interceptions_total", dto.MetricType_COUNTER, "Intercepted requests.", "provider"),
	}
	sections := []documentSection{
		{name: "common", filter: metricFilter{prefixes: []string{"coder_ai_gateway_"}}},
	}

	_, err := updateDocumentSections(doc, metrics, testDocPath, sections)
	require.ErrorContains(t, err, "closing marker with no opening marker")
}

// A second closing marker leaves the text between it and the pair it follows
// inside no section at all.
func TestUpdateDocumentSectionsRejectsDuplicateClosingMarker(t *testing.T) {
	t.Parallel()

	prefix := string(namedGeneratorPrefix(testDocPath, "common"))
	suffix := string(namedGeneratorSuffix(testDocPath, "common"))
	doc := []byte(prefix + "\nstale\n" + suffix + "\nbetween\n" + suffix + "\n")
	metrics := []*dto.MetricFamily{
		metricFamily("coder_ai_gateway_interceptions_total", dto.MetricType_COUNTER, "Intercepted requests.", "provider"),
	}
	sections := []documentSection{
		{name: "common", filter: metricFilter{prefixes: []string{"coder_ai_gateway_"}}},
	}

	_, err := updateDocumentSections(doc, metrics, testDocPath, sections)
	require.ErrorContains(t, err, "2 closing markers for the same section")
}

// A document that carries the same section twice would keep every pair after
// the first stale, because only the first pair is rewritten.
func TestUpdateDocumentSectionsRejectsDuplicateSection(t *testing.T) {
	t.Parallel()

	prefix := string(namedGeneratorPrefix(testDocPath, "common"))
	suffix := string(namedGeneratorSuffix(testDocPath, "common"))
	pair := prefix + "\nstale\n" + suffix + "\n"
	doc := []byte(pair + pair)
	metrics := []*dto.MetricFamily{
		metricFamily("coder_ai_gateway_interceptions_total", dto.MetricType_COUNTER, "Intercepted requests.", "provider"),
	}
	sections := []documentSection{
		{name: "common", filter: metricFilter{prefixes: []string{"coder_ai_gateway_"}}},
	}

	_, err := updateDocumentSections(doc, metrics, testDocPath, sections)
	require.ErrorContains(t, err, "2 generated markers for the same section")
}

// A marker naming another make target is never rewritten, and its section name
// alone would make it look claimed.
func TestUpdateDocumentSectionsRejectsForeignTargetMarker(t *testing.T) {
	t.Parallel()

	prefix := string(namedGeneratorPrefix(testDocPath, "common"))
	suffix := string(namedGeneratorSuffix(testDocPath, "common"))
	foreignPrefix := string(namedGeneratorPrefix("docs/other/target.md", "common"))
	foreignSuffix := string(namedGeneratorSuffix("docs/other/target.md", "common"))
	doc := []byte(prefix + "\nstale\n" + suffix + "\n" + foreignPrefix + "\nstale foreign\n" + foreignSuffix + "\n")
	metrics := []*dto.MetricFamily{
		metricFamily("coder_ai_gateway_interceptions_total", dto.MetricType_COUNTER, "Intercepted requests.", "provider"),
	}
	sections := []documentSection{
		{name: "common", filter: metricFilter{prefixes: []string{"coder_ai_gateway_"}}},
	}

	_, err := updateDocumentSections(doc, metrics, testDocPath, sections)
	require.ErrorContains(t, err, `names make target "docs/other/target.md"`)
	// The opener literal pins the check to the opener branch, which shares its
	// error template with the closer branch.
	require.ErrorContains(t, err, strconv.Quote(foreignPrefix))
}

// A closing marker naming another make target is never rewritten either, and
// the opening marker of its pair can be the current target's own.
func TestUpdateDocumentSectionsRejectsForeignTargetClosingMarker(t *testing.T) {
	t.Parallel()

	prefix := string(namedGeneratorPrefix(testDocPath, "common"))
	suffix := string(namedGeneratorSuffix(testDocPath, "common"))
	foreignSuffix := string(namedGeneratorSuffix("docs/other/target.md", "common"))
	doc := []byte(prefix + "\nstale\n" + suffix + "\n" + foreignSuffix + "\n")
	metrics := []*dto.MetricFamily{
		metricFamily("coder_ai_gateway_interceptions_total", dto.MetricType_COUNTER, "Intercepted requests.", "provider"),
	}
	sections := []documentSection{
		{name: "common", filter: metricFilter{prefixes: []string{"coder_ai_gateway_"}}},
	}

	_, err := updateDocumentSections(doc, metrics, testDocPath, sections)
	require.ErrorContains(t, err, `names make target "docs/other/target.md"`)
	// The full closer literal is what separates this branch from the opener-side
	// check, which cannot name a closing marker.
	require.ErrorContains(t, err, strconv.Quote(foreignSuffix))
}

// The unnamed pair belongs to default mode, so --section mode never rewrites it.
func TestUpdateDocumentSectionsRejectsDefaultMarker(t *testing.T) {
	t.Parallel()

	prefix := string(namedGeneratorPrefix(testDocPath, "common"))
	suffix := string(namedGeneratorSuffix(testDocPath, "common"))
	defaultPrefix := string(defaultGeneratorPrefix(testDocPath))
	defaultSuffix := string(defaultGeneratorSuffix(testDocPath))
	doc := []byte(prefix + "\nstale\n" + suffix + "\n" + defaultPrefix + "\nstale default\n" + defaultSuffix + "\n")
	metrics := []*dto.MetricFamily{
		metricFamily("coder_ai_gateway_interceptions_total", dto.MetricType_COUNTER, "Intercepted requests.", "provider"),
	}
	sections := []documentSection{
		{name: "common", filter: metricFilter{prefixes: []string{"coder_ai_gateway_"}}},
	}

	_, err := updateDocumentSections(doc, metrics, testDocPath, sections)
	require.ErrorContains(t, err, "unnamed generated section")
}

// Default mode rewrites the unnamed pair only, so a named pair left in the
// document is as stale as an unclaimed section is in --section mode.
func TestUpdateDefaultSectionRejectsNamedMarker(t *testing.T) {
	t.Parallel()

	prefix := string(defaultGeneratorPrefix(testDocPath))
	suffix := string(defaultGeneratorSuffix(testDocPath))
	stalePrefix := string(namedGeneratorPrefix(testDocPath, "stale"))
	staleSuffix := string(namedGeneratorSuffix(testDocPath, "stale"))
	doc := []byte(stalePrefix + "\nstale named\n" + staleSuffix + "\n" + prefix + "\nstale\n" + suffix + "\n")
	metrics := []*dto.MetricFamily{
		metricFamily("coder_metric_total", dto.MetricType_COUNTER, "Requests.", "provider"),
	}

	_, err := updateDefaultSection(doc, metrics, testDocPath)
	require.ErrorContains(t, err, `document has generated section "stale", which default mode never rewrites`)
}

// Two unnamed pairs leave everything after the first stale.
func TestUpdateDefaultSectionRejectsDuplicateMarkers(t *testing.T) {
	t.Parallel()

	prefix := string(defaultGeneratorPrefix(testDocPath))
	suffix := string(defaultGeneratorSuffix(testDocPath))
	pair := prefix + "\nstale\n" + suffix + "\n"
	doc := []byte(pair + pair)
	metrics := []*dto.MetricFamily{
		metricFamily("coder_metric_total", dto.MetricType_COUNTER, "Requests.", "provider"),
	}

	_, err := updateDefaultSection(doc, metrics, testDocPath)
	require.ErrorContains(t, err, "2 generated markers for the same section")
}

func TestUpdateDefaultSectionPreservesDefaultMarkers(t *testing.T) {
	t.Parallel()

	const defaultDocPath = "docs/admin/integrations/prometheus.md"
	prefix := string(defaultGeneratorPrefix(defaultDocPath))
	suffix := string(defaultGeneratorSuffix(defaultDocPath))
	doc := []byte("before\n" + prefix + "\nstale\n" + suffix + "\nafter\n")
	metrics := []*dto.MetricFamily{
		metricFamily("z_metric", dto.MetricType_GAUGE, "Last.", "z", "a"),
		metricFamily("a_metric", dto.MetricType_COUNTER, "First."),
	}

	updated, err := updateDefaultSection(doc, metrics, defaultDocPath)
	require.NoError(t, err)
	require.Contains(t, string(updated), prefix)
	require.Contains(t, string(updated), suffix)
	require.NotContains(t, string(updated), "stale")
	require.Less(t, bytes.Index(updated, []byte("z_metric")), bytes.Index(updated, []byte(suffix)))
	require.Contains(t, string(updated), "| `z_metric` | gauge | Last. | `a` `z` |")
	// A metric without labels renders an empty Labels cell.
	require.Contains(t, string(updated), "| `a_metric` | counter | First. |  |")
}

func TestRenderMetricTableEscapesCellSeparators(t *testing.T) {
	t.Parallel()

	table := string(renderMetricTable([]*dto.MetricFamily{
		metricFamily("coder_metric_total", dto.MetricType_COUNTER, "Requests by a|b.\nSecond line.", "provider"),
	}))
	require.Contains(t, table, "| `coder_metric_total` | counter | Requests by a\\|b. Second line. | `provider` |")
}

func sectionBody(t *testing.T, doc []byte, prefix, suffix string) string {
	t.Helper()

	start := bytes.Index(doc, []byte(prefix))
	require.GreaterOrEqual(t, start, 0)
	start += len(prefix)
	end := bytes.Index(doc[start:], []byte(suffix))
	require.GreaterOrEqual(t, end, 0)
	return string(doc[start : start+end])
}

func metricFamily(name string, metricType dto.MetricType, help string, labels ...string) *dto.MetricFamily {
	metric := &dto.Metric{}
	for _, label := range labels {
		labelValue := ""
		metric.Label = append(metric.Label, &dto.LabelPair{Name: &label, Value: &labelValue})
	}
	return &dto.MetricFamily{
		Name:   &name,
		Type:   &metricType,
		Help:   &help,
		Metric: []*dto.Metric{metric},
	}
}

func metricNames(metrics []*dto.MetricFamily) []string {
	names := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		names = append(names, metric.GetName())
	}
	return names
}
