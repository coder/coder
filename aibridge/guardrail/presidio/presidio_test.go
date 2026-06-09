package presidio_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/guardrail"
	"github.com/coder/coder/v2/aibridge/guardrail/presidio"
)

// mockPresidio is a minimal stand-in for the analyzer + anonymizer services.
// /analyze flags an EMAIL_ADDRESS span when the text contains "@" and a US_SSN
// span when it contains "ssn"; /anonymize replaces every analyzer span with the
// configured new_value.
func mockPresidio(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/analyze", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		var out []map[string]any
		if i := strings.Index(req.Text, "@"); i >= 0 {
			// Expand to the surrounding token for a realistic span.
			start := strings.LastIndex(req.Text[:i], " ") + 1
			end := i
			for end < len(req.Text) && req.Text[end] != ' ' {
				end++
			}
			out = append(out, map[string]any{
				"entity_type": "EMAIL_ADDRESS", "start": start, "end": end, "score": 0.9,
			})
		}
		if i := strings.Index(strings.ToLower(req.Text), "ssn"); i >= 0 {
			out = append(out, map[string]any{
				"entity_type": "US_SSN", "start": i, "end": i + 3, "score": 0.95,
			})
		}
		require.NoError(t, json.NewEncoder(w).Encode(out))
	})
	mux.HandleFunc("/anonymize", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text        string `json:"text"`
			Anonymizers map[string]struct {
				NewValue string `json:"new_value"`
			} `json:"anonymizers"`
			Results []struct {
				Start int `json:"start"`
				End   int `json:"end"`
			} `json:"analyzer_results"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		newValue := req.Anonymizers["DEFAULT"].NewValue

		// Replace spans right-to-left so earlier offsets stay valid.
		text := req.Text
		results := req.Results
		for i := len(results) - 1; i >= 0; i-- {
			s, e := results[i].Start, results[i].End
			if s < 0 || e > len(text) || s > e {
				continue
			}
			text = text[:s] + newValue + text[e:]
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"text": text}))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestPresidio_Mask(t *testing.T) {
	t.Parallel()
	srv := mockPresidio(t)

	g, err := presidio.New(presidio.Config{
		Name:          "presidio",
		AnalyzerURL:   srv.URL,
		AnonymizerURL: srv.URL,
		EntityActions: map[string]presidio.EntityAction{
			"EMAIL_ADDRESS": presidio.ActionMask,
		},
		RedactValue: "<REDACTED>",
	})
	require.NoError(t, err)

	res, err := g.Evaluate(context.Background(), guardrail.Request{
		Texts: []guardrail.TextRef{
			{Pointer: "messages.0.content", Value: "my email is ishaan@berri.ai"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, guardrail.ActionAllow, res.Action)
	require.Len(t, res.Edits, 1)
	require.Equal(t, "messages.0.content", res.Edits[0].Pointer)
	require.Equal(t, "my email is <REDACTED>", res.Edits[0].Value)

	entities, _ := res.Annotations["entities"].(map[string]int)
	require.Equal(t, 1, entities["EMAIL_ADDRESS"])
	require.Equal(t, false, res.Annotations["blocked"])
}

func TestPresidio_Block(t *testing.T) {
	t.Parallel()
	srv := mockPresidio(t)

	g, err := presidio.New(presidio.Config{
		Name:        "presidio",
		AnalyzerURL: srv.URL,
		EntityActions: map[string]presidio.EntityAction{
			"US_SSN": presidio.ActionBlock,
		},
	})
	require.NoError(t, err)

	res, err := g.Evaluate(context.Background(), guardrail.Request{
		Texts: []guardrail.TextRef{
			{Pointer: "messages.0.content", Value: "my ssn is on file"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, guardrail.ActionBlock, res.Action)
	require.NotEmpty(t, res.Reason)
	require.Empty(t, res.Edits)
	require.Equal(t, true, res.Annotations["blocked"])
}

func TestPresidio_ThresholdFiltersOut(t *testing.T) {
	t.Parallel()
	srv := mockPresidio(t)

	// The mock scores EMAIL_ADDRESS at 0.9; a threshold above that drops it.
	g, err := presidio.New(presidio.Config{
		Name:          "presidio",
		AnalyzerURL:   srv.URL,
		AnonymizerURL: srv.URL,
		EntityActions: map[string]presidio.EntityAction{
			"EMAIL_ADDRESS": presidio.ActionMask,
		},
		ScoreThresholds: map[string]float64{"EMAIL_ADDRESS": 0.95},
	})
	require.NoError(t, err)

	res, err := g.Evaluate(context.Background(), guardrail.Request{
		Texts: []guardrail.TextRef{
			{Pointer: "messages.0.content", Value: "my email is ishaan@berri.ai"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, guardrail.ActionAllow, res.Action)
	require.Empty(t, res.Edits)
	require.Nil(t, res.Annotations)
}

func TestPresidio_ValidationErrors(t *testing.T) {
	t.Parallel()
	_, err := presidio.New(presidio.Config{Name: "p", AnalyzerURL: "http://x", EntityActions: map[string]presidio.EntityAction{"EMAIL_ADDRESS": presidio.ActionMask}})
	require.ErrorContains(t, err, "anonymizer URL required")

	_, err = presidio.New(presidio.Config{Name: "p", AnalyzerURL: "http://x", EntityActions: map[string]presidio.EntityAction{"X": "NOPE"}})
	require.ErrorContains(t, err, "unknown action")
}
