package entity

import (
	"context"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
)

// LifecycleEntryLimit is the largest number of entries a read will return for
// one entity.
//
// It is not pagination and is not tuned to anything. An entity's lifecycle is
// a state machine without cycles, so the entries one entity can accumulate are
// bounded by the sequences that machine allows, and that bound is far below
// this number. The limit exists to catch the case where that reasoning has
// stopped being true.
//
// The reasoning holds only because reads are per subject. A query gathering
// what one actor did would span many subjects and have no such bound, so it
// could not borrow this limit.
const LifecycleEntryLimit = 10000

// ErrTooManyEntries is returned when one entity has more entries than its
// lifecycle can account for.
//
// This is not an ordinary error and callers should not treat it as one. It
// means either that something is writing entries which should not be, or that
// the lifecycle model no longer describes what the system does. Neither is
// recoverable in the caller, and both need a person.
var ErrTooManyEntries = xerrors.New("more lifecycle entries than an entity's lifecycle can produce")

// LifecycleEntriesBySubject returns every entry recording something that
// happened to subject, oldest first. It fails rather than truncating when
// there are more than LifecycleEntryLimit of them.
//
// Nothing is returned in that case. Handing back the entries that did fit
// would be worse than failing: a partial account read as a whole one is how a
// reader reaches a wrong conclusion and holds it confidently.
//
// It also logs, because the caller's error is not enough. Whoever called this
// may do nothing with the error, and this condition needs to reach someone who
// can look at the journal. Post proof of concept this should raise something
// an operator already watches, a metric or an alert, since a log line is
// carried only as far as somebody reads it.
func LifecycleEntriesBySubject(ctx context.Context, log slog.Logger, store database.Store, subject Ref) ([]database.EntityJournal, error) {
	if !subject.Type.Valid() {
		return nil, xerrors.Errorf("subject type %q names no kind of entity", subject.Type)
	}

	// One more than the limit, so that receiving that many says the set was
	// larger rather than exactly this size.
	entries, err := store.GetLifecycleEntriesBySubject(ctx, database.GetLifecycleEntriesBySubjectParams{
		SubjectType: string(subject.Type),
		Subject:     subject.ID,
		Limit:       LifecycleEntryLimit + 1,
	})
	if err != nil {
		return nil, xerrors.Errorf("read lifecycle entries by subject: %w", err)
	}

	if len(entries) > LifecycleEntryLimit {
		log.Error(ctx, "lifecycle journal holds more entries than an entity can account for",
			slog.F("entity_type", string(subject.Type)),
			slog.F("entity_id", subject.ID),
			slog.F("limit", LifecycleEntryLimit))

		return nil, xerrors.Errorf("subject %s %s: %w", subject.Type, subject.ID, ErrTooManyEntries)
	}

	return entries, nil
}
