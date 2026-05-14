package main

import (
	"errors"
	"testing"

	codernats "github.com/coder/coder/v2/coderd/x/nats"
)

func TestPlanSubjectsSingleSubjectMatchesLegacy(t *testing.T) {
	t.Parallel()
	// With numSubjects=1, every pub and sub maps to subject 0 and
	// every subscriber expects every published message, reproducing
	// the historical single-subject natsbench behavior exactly.
	plan, err := planSubjects(3, 4, 1, 1000, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	if plan.NumSubjects != 1 {
		t.Fatalf("NumSubjects = %d, want 1", plan.NumSubjects)
	}
	for i, s := range plan.PubSubject {
		if s != 0 {
			t.Errorf("PubSubject[%d] = %d, want 0", i, s)
		}
	}
	for j, s := range plan.SubSubject {
		if s != 0 {
			t.Errorf("SubSubject[%d] = %d, want 0", j, s)
		}
	}
	// perPub = 1000/3 = 333, rem = 1; pub 0 gets 334, pubs 1,2 get 333.
	want := []int{334, 333, 333}
	for i, n := range plan.PerPubMsgs {
		if n != want[i] {
			t.Errorf("PerPubMsgs[%d] = %d, want %d", i, n, want[i])
		}
	}
	if plan.TotalPublished != 1000 {
		t.Errorf("TotalPublished = %d, want 1000", plan.TotalPublished)
	}
	for j, e := range plan.ExpectPerSub {
		if e != 1000 {
			t.Errorf("ExpectPerSub[%d] = %d, want 1000", j, e)
		}
	}
}

func TestPlanSubjectsMultiSubjectDistribution(t *testing.T) {
	t.Parallel()
	// 4 pubs, 6 subs, 2 subjects, 100 msgs (total, not per-pub).
	// PerPubMsgs: 100/4 = 25 each, no remainder.
	// pub 0,2 -> subject 0; pub 1,3 -> subject 1.
	// published per subject = 25+25 = 50 each.
	// sub 0,2,4 -> subject 0 (expect 50); sub 1,3,5 -> subject 1 (expect 50).
	plan, err := planSubjects(4, 6, 2, 100, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	if got, want := plan.TotalPublished, int64(100); got != want {
		t.Errorf("TotalPublished = %d, want %d", got, want)
	}
	wantPub := []int{0, 1, 0, 1}
	for i, s := range plan.PubSubject {
		if s != wantPub[i] {
			t.Errorf("PubSubject[%d] = %d, want %d", i, s, wantPub[i])
		}
	}
	wantSub := []int{0, 1, 0, 1, 0, 1}
	for j, s := range plan.SubSubject {
		if s != wantSub[j] {
			t.Errorf("SubSubject[%d] = %d, want %d", j, s, wantSub[j])
		}
	}
	for j, e := range plan.ExpectPerSub {
		if e != 50 {
			t.Errorf("ExpectPerSub[%d] = %d, want 50", j, e)
		}
	}
}

func TestPlanSubjectsPerPubSymmetric(t *testing.T) {
	t.Parallel()
	// Symmetric cluster modes: msgs is per-publisher. 3 pubs * 200
	// = 600 total published, split round-robin across 3 subjects so
	// each subject sees exactly 200 messages from one publisher.
	plan, err := planSubjects(3, 3, 3, 200, true)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	if got, want := plan.TotalPublished, int64(600); got != want {
		t.Errorf("TotalPublished = %d, want %d", got, want)
	}
	for i, n := range plan.PerPubMsgs {
		if n != 200 {
			t.Errorf("PerPubMsgs[%d] = %d, want 200", i, n)
		}
	}
	// Each sub assigned to one subject; each subject has exactly one
	// publisher emitting 200 msgs.
	for j, e := range plan.ExpectPerSub {
		if e != 200 {
			t.Errorf("ExpectPerSub[%d] = %d, want 200", j, e)
		}
	}
}

func TestPlanSubjectsSubjectsExceedPubs(t *testing.T) {
	t.Parallel()
	// 2 pubs, 4 subs, 4 subjects, 100 msgs total. pub 0 -> subject 0,
	// pub 1 -> subject 1. Subjects 2 and 3 have zero publishers, so
	// subscribers pinned to them expect zero messages.
	// Pub 0 carries the remainder: 100/2 = 50; pub 0 = 50, pub 1 = 50.
	plan, err := planSubjects(2, 4, 4, 100, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	want := []int64{50, 50, 0, 0}
	for j, e := range plan.ExpectPerSub {
		if e != want[j] {
			t.Errorf("ExpectPerSub[%d] = %d, want %d", j, e, want[j])
		}
	}
}

func TestPlanSubjectsZeroSubs(t *testing.T) {
	t.Parallel()
	plan, err := planSubjects(2, 0, 3, 60, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	if len(plan.SubSubject) != 0 || len(plan.ExpectPerSub) != 0 {
		t.Fatalf("expected empty subscriber slices, got SubSubject=%v ExpectPerSub=%v",
			plan.SubSubject, plan.ExpectPerSub)
	}
	if plan.TotalPublished != 60 {
		t.Errorf("TotalPublished = %d, want 60", plan.TotalPublished)
	}
}

func TestPlanSubjectsInvalidArgs(t *testing.T) {
	t.Parallel()
	if _, err := planSubjects(1, 1, 0, 10, false); err == nil {
		t.Errorf("planSubjects with numSubjects=0 returned nil error")
	}
	if _, err := planSubjects(-1, 1, 1, 10, false); err == nil {
		t.Errorf("planSubjects with pubs=-1 returned nil error")
	}
	if _, err := planSubjects(1, -1, 1, 10, false); err == nil {
		t.Errorf("planSubjects with subs=-1 returned nil error")
	}
	if _, err := planSubjects(1, 1, 1, -1, false); err == nil {
		t.Errorf("planSubjects with msgs=-1 returned nil error")
	}
}

func TestNativeSubjectNaming(t *testing.T) {
	t.Parallel()
	if got := nativeSubject("bench", 0, 1); got != "bench" {
		t.Errorf("single-subject native = %q, want %q", got, "bench")
	}
	if got := nativeSubject("bench", 0, 4); got != "bench.0" {
		t.Errorf("multi native[0] = %q, want %q", got, "bench.0")
	}
	if got := nativeSubject("bench", 3, 4); got != "bench.3" {
		t.Errorf("multi native[3] = %q, want %q", got, "bench.3")
	}
}

func TestCoderSubjectNamingValid(t *testing.T) {
	t.Parallel()
	if got := coderSubject("bench", 0, 1); got != "bench" {
		t.Errorf("single-subject coder = %q, want %q", got, "bench")
	}
	// Every generated coder subject must be a valid legacy event so
	// it can be mapped to a NATS subject by the wrapper without
	// surprising the operator at run time.
	subjects := buildCoderSubjects("bench", 5)
	if len(subjects) != 5 {
		t.Fatalf("buildCoderSubjects len = %d, want 5", len(subjects))
	}
	for i, s := range subjects {
		if _, err := codernats.LegacyEventSubject(s); err != nil {
			t.Errorf("subjects[%d]=%q: LegacyEventSubject error: %v", i, s, err)
			if !errors.Is(err, codernats.ErrInvalidToken) && !errors.Is(err, codernats.ErrInvalidSubject) {
				// Sanity check the error category in case the
				// validation rules expand later; we want this test
				// to fail loudly rather than silently allow new
				// invalid characters.
				t.Errorf("unexpected error kind for %q: %v", s, err)
			}
		}
	}
}
