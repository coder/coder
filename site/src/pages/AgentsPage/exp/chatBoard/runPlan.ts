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
	const moves = (plan.projectMoves ?? []).map(({ chat, projectId }) =>
		deps.moveProject(chat.id, projectId),
	);
	const movesBefore = (plan.projectMoves ?? []).map(({ chat }) => ({
		chatId: chat.id,
		projectId: chat.project_id ?? null,
	}));
	const moved = moves.length > 0 ? Promise.all(moves) : undefined;
	const received =
		plan.receiver &&
		deps.write(plan.receiver.chat.id, plan.receiver.labels, moved);
	const pending = [
		...moves,
		...(received ? [received] : []),
		...plan.writes.map(({ chat, labels }) =>
			deps.write(chat.id, labels, received ?? moved),
		),
		...(plan.titles ?? []).map(({ chat, title }) =>
			deps.rename(chat.id, title),
		),
	];
	const undo = () => {
		const movesBack = movesBefore.map(({ chatId, projectId }) =>
			deps.moveProject(chatId, projectId),
		);
		const movedBack = movesBack.length > 0 ? Promise.all(movesBack) : undefined;
		const restores = writesBefore.map(({ chatId, labels }) =>
			deps.write(chatId, labels, movedBack),
		);
		const restored = Promise.all(restores);
		// `restored` is listed as well as gating the receiver: without a
		// receiver, or if that write never starts, nothing else would observe
		// its rejection.
		const all = [
			...movesBack,
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
