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
func LifecycleEntriesBySubject(ctx context.Context, log slog.Logger, store database.Store, subject Ref) ([]database.EntityJournal, error) {
	if !subject.Type.Valid() {
		return nil, xerrors.Errorf("subject type %q names no kind of entity", subject.Type)
	}

	entries, err := store.GetLifecycleEntriesBySubject(ctx, database.GetLifecycleEntriesBySubjectParams{
		SubjectType: string(subject.Type),
		Subject:     subject.ID,
		Limit:       LifecycleEntryLimit + 1,
	})
	if err != nil {
		return nil, xerrors.Errorf("read lifecycle entries by subject: %w", err)
	}

	return checkCount(ctx, log, entries, "subject", subject)
}

// LifecycleEntriesByActor returns every entry recording something actor
// brought about, oldest first.
func LifecycleEntriesByActor(ctx context.Context, log slog.Logger, store database.Store, actor Ref) ([]database.EntityJournal, error) {
	if !actor.Type.Valid() {
		return nil, xerrors.Errorf("actor type %q names no kind of entity", actor.Type)
	}

	entries, err := store.GetLifecycleEntriesByActor(ctx, database.GetLifecycleEntriesByActorParams{
		ActorType: string(actor.Type),
		Actor:     actor.ID,
		Limit:     LifecycleEntryLimit + 1,
	})
	if err != nil {
		return nil, xerrors.Errorf("read lifecycle entries by actor: %w", err)
	}

	return checkCount(ctx, log, entries, "actor", actor)
}

// checkCount fails the read when the journal holds more entries for one entity
// than its lifecycle can produce.
//
// The queries ask for one entry more than the limit, so receiving that many
// means there were at least that many, not exactly.
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
func checkCount(ctx context.Context, log slog.Logger, entries []database.EntityJournal, role string, ref Ref) ([]database.EntityJournal, error) {
	if len(entries) <= LifecycleEntryLimit {
		return entries, nil
	}

	log.Error(ctx, "lifecycle journal holds more entries than an entity can account for",
		slog.F("role", role),
		slog.F("entity_type", string(ref.Type)),
		slog.F("entity_id", ref.ID),
		slog.F("limit", LifecycleEntryLimit))

	return nil, xerrors.Errorf("%s %s %s: %w", role, ref.Type, ref.ID, ErrTooManyEntries)
}
