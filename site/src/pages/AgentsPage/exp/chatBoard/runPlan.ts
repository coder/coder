import { toast } from "sonner";
import type { Plan } from "./boardApi";
import type { BoardStorage } from "./boardStorage";

// Long enough to notice an accidental drop on a phone and reach the toast.
const UNDO_MS = 10_000;

/** What a plan needs from the page: the two mutations and the storage setter. */
export type PlanDeps = {
	readonly write: (
		chatId: string,
		labels: Record<string, string>,
	) => Promise<unknown>;
	readonly rename: (chatId: string, title: string) => Promise<unknown>;
	readonly updateStorage: (patch: Partial<BoardStorage>) => void;
};

// The mutations report their own errors with a toast; a rejection reaching
// the caller would only add an unhandled-rejection report.
const reported = (writes: Promise<unknown>): Promise<void> =>
	writes.then(
		() => undefined,
		() => undefined,
	);

/**
 * Applies a Plan: storage patch, label writes, title writes, and the undo
 * toast when the plan asks for it. Decides nothing about what an action
 * means. Writes go out independently; a partial failure leaves the other
 * chats where they were and skips the undo offer. Never rejects.
 */
export const runPlan = (plan: Plan | null, deps: PlanDeps): Promise<void> => {
	if (!plan) return Promise.resolve();
	if (plan.storage) deps.updateStorage(plan.storage);
	// Snapshot before sending: undo restores the maps as they were, not as
	// the optimistic cache patch left them.
	const before = plan.writes.map(({ chat }) => ({
		chatId: chat.id,
		labels: chat.labels,
	}));
	const pending = [
		...plan.writes.map(({ chat, labels }) => deps.write(chat.id, labels)),
		...(plan.titles ?? []).map(({ chat, title }) =>
			deps.rename(chat.id, title),
		),
	];
	return reported(
		Promise.all(pending).then(() => {
			if (!plan.undo) return;
			toast(plan.undo, {
				duration: UNDO_MS,
				action: {
					label: "Undo",
					onClick: () =>
						void reported(
							Promise.all(
								before.map(({ chatId, labels }) => deps.write(chatId, labels)),
							),
						),
				},
			});
		}),
	);
};
