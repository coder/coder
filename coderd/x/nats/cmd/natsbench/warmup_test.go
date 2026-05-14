package main

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"golang.org/x/xerrors"
)

func TestWarmupPayloadIsTagged(t *testing.T) {
	t.Parallel()
	p := warmupPayload(3)
	if len(p) != 2 {
		t.Fatalf("warmup payload len = %d, want 2", len(p))
	}
	if !isWarmupPayload(p) {
		t.Errorf("expected warmup payload to be detected as warmup")
	}
	if warmupReplicaIdx(p) != 3 {
		t.Errorf("warmupReplicaIdx = %d, want 3", warmupReplicaIdx(p))
	}
	// A hot payload uses payload[i]=byte(i), so byte[0]==0.
	hot := []byte{0, 1, 2, 3, 4}
	if isWarmupPayload(hot) {
		t.Errorf("hot payload misdetected as warmup")
	}
}

func TestWarmupReplicaIdxOutOfRange(t *testing.T) {
	t.Parallel()
	// Out-of-range replica indices are coerced to 0; the corresponding
	// expectedWarmupMask falls back to bit 0 ("any one warmup observed").
	p := warmupPayload(300)
	if warmupReplicaIdx(p) != 0 {
		t.Errorf("warmupReplicaIdx = %d, want 0", warmupReplicaIdx(p))
	}
}

func TestExpectedWarmupMaskSinglePublisherPerSubject(t *testing.T) {
	t.Parallel()
	// 4 pubs, 8 subs, 4 subjects, replicas=4: pubReplicaOf(i)=i so each
	// subject has exactly one publisher on its own replica.
	plan, err := planSubjects(4, 8, 4, 100, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	masks := expectedWarmupMask(plan, func(i int) int { return i })
	want := []uint64{1 << 0, 1 << 1, 1 << 2, 1 << 3}
	if !reflect.DeepEqual(masks, want) {
		t.Errorf("masks = %v, want %v", masks, want)
	}
}

func TestExpectedWarmupMaskMultipleReplicasPerSubject(t *testing.T) {
	t.Parallel()
	// 6 pubs, 4 subjects, replicas=3, pubReplicaOf(i)=i%3.
	// Publishers 0,4 -> subject 0 on replicas 0,1.
	// Publishers 1,5 -> subject 1 on replicas 1,2.
	// Publisher 2   -> subject 2 on replica 2.
	// Publisher 3   -> subject 3 on replica 0.
	plan, err := planSubjects(6, 4, 4, 600, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	masks := expectedWarmupMask(plan, func(i int) int { return i % 3 })
	want := []uint64{
		(1 << 0) | (1 << 1),
		(1 << 1) | (1 << 2),
		(1 << 2),
		(1 << 0),
	}
	if !reflect.DeepEqual(masks, want) {
		t.Errorf("masks = %v, want %v", masks, want)
	}
}

func TestExpectedWarmupMaskZeroForUnpublishedSubject(t *testing.T) {
	t.Parallel()
	// 2 pubs, 4 subjects: subjects 2,3 have no publishers so mask must be 0.
	plan, err := planSubjects(2, 4, 4, 100, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	masks := expectedWarmupMask(plan, func(int) int { return 0 })
	if masks[2] != 0 {
		t.Errorf("subject 2 mask = %#x, want 0", masks[2])
	}
	if masks[3] != 0 {
		t.Errorf("subject 3 mask = %#x, want 0", masks[3])
	}
}

func TestWarmupStateMarkAndSatisfied(t *testing.T) {
	t.Parallel()
	var w warmupState
	mask := uint64((1 << 0) | (1 << 2))
	if w.satisfied(mask) {
		t.Errorf("empty state must not satisfy non-zero mask")
	}
	w.mark(0)
	if w.satisfied(mask) {
		t.Errorf("after only replica 0 seen, not yet satisfied")
	}
	w.mark(2)
	if !w.satisfied(mask) {
		t.Errorf("after replicas 0,2 seen, must be satisfied")
	}
	if !w.satisfied(0) {
		t.Errorf("zero mask must always be satisfied")
	}
	w.reset()
	if w.satisfied(mask) {
		t.Errorf("reset must clear the seen bitmask")
	}
}

func TestPubReplicasPerSubject(t *testing.T) {
	t.Parallel()
	plan, err := planSubjects(6, 6, 3, 60, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	got := pubReplicasPerSubject(plan, func(i int) int { return i % 3 })
	// pubs i=0..5 are on subjects i%3 = 0,1,2,0,1,2 and replicas i%3 = 0,1,2,0,1,2.
	// So per subject the publishers come from exactly one replica.
	want := [][]int{{0}, {1}, {2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pubReplicasPerSubject = %v, want %v", got, want)
	}
}

// fakeWarmupRunner records (subject, replica) publish calls so we can
// assert the warmup loop sends what we expect.
type fakeWarmupRunner struct {
	mu        sync.Mutex
	publishes []struct {
		subject string
		replica int
	}
	flushes []int
	// markOnPublish, when non-nil, is invoked synchronously inside
	// publishWarmup; the test uses it to mark the corresponding
	// warmupState so the loop terminates.
	markOnPublish func(subject string, replica int)
	publishErr    error
}

func (f *fakeWarmupRunner) publishWarmup(subject string, replica int) error {
	f.mu.Lock()
	f.publishes = append(f.publishes, struct {
		subject string
		replica int
	}{subject, replica})
	f.mu.Unlock()
	if f.markOnPublish != nil {
		f.markOnPublish(subject, replica)
	}
	return f.publishErr
}

func (f *fakeWarmupRunner) flushReplica(replica int) error {
	f.mu.Lock()
	f.flushes = append(f.flushes, replica)
	f.mu.Unlock()
	return nil
}

func TestWarmupSubjectsBlockingHappyPath(t *testing.T) {
	t.Parallel()
	subjects := []string{"bench_0", "bench_1"}
	expected := []uint64{1 << 0, 1 << 1}
	subscriberSubject := []int{0, 0, 1, 1}
	subs := []*warmupState{{}, {}, {}, {}}
	pubReplicasBySubj := [][]int{{0}, {1}}
	r := &fakeWarmupRunner{}
	r.markOnPublish = func(subj string, rep int) {
		// Mark every subscriber on the subject as having seen replica
		// rep. In a real run this would happen via the subscriber
		// callback.
		for j, s := range subscriberSubject {
			if subjects[s] == subj {
				subs[j].mark(rep)
			}
		}
	}
	err := warmupSubjectsBlocking(subjects, expected, subscriberSubject, subs, pubReplicasBySubj, r, time.Now().Add(time.Second), 5*time.Millisecond)
	if err != nil {
		t.Fatalf("warmupSubjectsBlocking: %v", err)
	}
	if len(r.publishes) != 2 {
		t.Errorf("expected 2 warmup publishes, got %v", r.publishes)
	}
	for _, s := range subs {
		// reset wipes the state so every subscriber's saw-mask is zero;
		// satisfied(0) is always true so this is a structural assertion.
		if !s.satisfied(0) {
			t.Errorf("zero mask must always be satisfied after reset")
		}
	}
}

func TestWarmupSubjectsBlockingPublishError(t *testing.T) {
	t.Parallel()
	subjects := []string{"bench_0"}
	expected := []uint64{1 << 0}
	subscriberSubject := []int{0}
	subs := []*warmupState{{}}
	pubReplicasBySubj := [][]int{{0}}
	sentinel := xerrors.New("boom")
	r := &fakeWarmupRunner{publishErr: sentinel}
	err := warmupSubjectsBlocking(subjects, expected, subscriberSubject, subs, pubReplicasBySubj, r, time.Now().Add(time.Second), 5*time.Millisecond)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestWarmupSubjectsBlockingSoftCapNoError(t *testing.T) {
	t.Parallel()
	// Subscribers never get marked. The loop must return nil after
	// the deadline (soft cap) instead of hanging.
	subjects := []string{"bench_0"}
	expected := []uint64{1 << 0}
	subscriberSubject := []int{0}
	subs := []*warmupState{{}}
	pubReplicasBySubj := [][]int{{0}}
	r := &fakeWarmupRunner{} // no markOnPublish
	err := warmupSubjectsBlocking(subjects, expected, subscriberSubject, subs, pubReplicasBySubj, r, time.Now().Add(20*time.Millisecond), 5*time.Millisecond)
	if err != nil {
		t.Fatalf("soft cap must return nil, got %v", err)
	}
	if len(r.publishes) == 0 {
		t.Errorf("expected at least one warmup publish before soft cap, got none")
	}
}

func TestWarmupSubjectsBlockingSkipsUnpublishedSubjects(t *testing.T) {
	t.Parallel()
	// Subject 1 has no publishers (mask 0). The loop must not try to
	// publish to it and must not consider its (nonexistent) subscriber
	// state.
	subjects := []string{"bench_0", "bench_1"}
	expected := []uint64{1 << 0, 0}
	subscriberSubject := []int{0, 1}
	subs := []*warmupState{{}, {}}
	pubReplicasBySubj := [][]int{{0}, nil}
	r := &fakeWarmupRunner{}
	r.markOnPublish = func(subj string, rep int) {
		for j, s := range subscriberSubject {
			if subjects[s] == subj {
				subs[j].mark(rep)
			}
		}
	}
	err := warmupSubjectsBlocking(subjects, expected, subscriberSubject, subs, pubReplicasBySubj, r, time.Now().Add(time.Second), 5*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	for _, p := range r.publishes {
		if p.subject == "bench_1" {
			t.Errorf("must not publish on unpublished subject, got %+v", r.publishes)
		}
	}
}
