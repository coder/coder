import { toast } from "#/components/Toaster/toast";
import type { Plan, Write } from "./boardApi";
import type { BoardStorage } from "./boardStorage";

// Long enough to notice an accidental drop on a phone and reach the toast.
const UNDO_MS = 10_000;

/** What a plan needs from the page: the two mutations and the storage setter. */
export type PlanDeps = {
	/** `after` gates the request: the write is skipped when it rejects. */
	readonly write: (
		chatId: string,
		labels: Record<string, string>,
		after?: Promise<unknown>,
	) => Promise<unknown>;
	readonly rename: (chatId: string, title: string) => Promise<unknown>;
	readonly moveProject: (
		chatId: string,
		projectId: string | null,
	) => Promise<unknown>;
	readonly updateStorage: (patch: Partial<BoardStorage>) => void;
};

// The mutations report their own errors with a toast; a rejection reaching
// the caller would only add an unhandled-rejection report.
const reported = (writes: Promise<unknown>): Promise<void> =>
	writes.then(
		() => undefined,
		() => undefined,
	);

// Snapshot before sending: undo restores the maps as they were, not as
// the optimistic cache patch left them.
const before = ({ chat }: Write) => ({ chatId: chat.id, labels: chat.labels });

type Move = Readonly<{ chatId: string; projectId: string | null }>;

/**
 * Sends the moves together and rejects if any failed, after moving the ones
 * that landed back to `previous`, so a card is never split across projects.
 * Undefined when there is nothing to move.
 */
const moveAll = (
	moves: readonly Move[],
	previous: readonly Move[],
	deps: PlanDeps,
): Promise<void> | undefined => {
	if (moves.length === 0) return undefined;
	return Promise.allSettled(
		moves.map(({ chatId, projectId }) => deps.moveProject(chatId, projectId)),
	).then((results) => {
		const failed = results.find((r) => r.status === "rejected");
		if (!failed) return;
		const back = previous.filter((_, i) => results[i]?.status === "fulfilled");
		void reported(
			Promise.all(
				back.map(({ chatId, projectId }) =>
					deps.moveProject(chatId, projectId),
				),
			),
		);
		throw failed.reason;
	});
};

/**
 * Applies a Plan: storage patch, project moves, label writes, title writes,
 * and the undo toast when the plan asks for it. Decides nothing about what an
 * action means. Project moves go first and every label write waits for them;
 * the receiver is next and the other writes wait for it, so a rejected step
 * leaves the later ones unsent. Undo restores in the reverse order. Resolves
 * true when every move, write and title landed; never rejects.
 */
export const runPlan = (
	plan: Plan | null,
	deps: PlanDeps,
): Promise<boolean> => {
	if (!plan) return Promise.resolve(true);
	if (plan.storage) deps.updateStorage(plan.storage);
	const receiverBefore = plan.receiver && before(plan.receiver);
	const writesBefore = plan.writes.map(before);
	const moves = (plan.projectMoves ?? []).map(({ chat, projectId }) => ({
		chatId: chat.id,
		projectId,
	}));
	const movesBefore = (plan.projectMoves ?? []).map(({ chat }) => ({
		chatId: chat.id,
		projectId: chat.project_id ?? null,
	}));
	const moved = moveAll(moves, movesBefore, deps);
	const received =
		plan.receiver &&
		deps.write(plan.receiver.chat.id, plan.receiver.labels, moved);
	// A skipped write resolves, so the sources also wait on the moves
	// themselves: a failed move must not strip notes the receiver never got.
	const sourcesAfter =
		moved || received ? Promise.all([moved, received]) : undefined;
	// Every gate is listed: a queued write only observes its gate once the
	// write starts, which can be after the gate has rejected.
	const pending = [
		...(moved ? [moved] : []),
		...(received ? [received] : []),
		...(sourcesAfter ? [sourcesAfter] : []),
		...plan.writes.map(({ chat, labels }) =>
			deps.write(chat.id, labels, sourcesAfter),
		),
		...(plan.titles ?? []).map(({ chat, title }) =>
			deps.rename(chat.id, title),
		),
	];
	const undo = () => {
		const movedBack = moveAll(movesBefore, moves, deps);
		const restores = writesBefore.map(({ chatId, labels }) =>
			deps.write(chatId, labels, movedBack),
		);
		const restored = Promise.all([movedBack, ...restores]);
		// `restored` is listed as well as gating the receiver: without a
		// receiver, or if that write never starts, nothing else would observe
		// its rejection.
		const all = [
			...restores,
			restored,
			...(receiverBefore
				? [deps.write(receiverBefore.chatId, receiverBefore.labels, restored)]
				: []),
		];
		return reported(Promise.all(all));
	};
	return Promise.all(pending).then(
		() => {
			if (plan.undo) {
				toast(plan.undo, {
					duration: UNDO_MS,
					action: { label: "Undo", onClick: () => void undo() },
				});
			}
			return true;
		},
		() => false,
	);
};
