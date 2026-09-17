import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { updateChatTitle } from "#/api/queries/chats";
import {
	addColumn,
	addNote,
	type BoardState,
	deleteColumn,
	detachChat,
	editNote,
	joinCard,
	mergeCards,
	moveCard,
	moveColumn,
	moveNote,
	type Plan,
	removeFromGroup,
	removeNote,
	renameCard,
	renameChat,
	renameColumn,
	setCardColor,
	type Write,
} from "./boardApi";
import type { BoardStorage } from "./boardStorage";
import { updateChatLabels } from "./updateChatLabels";

// Long enough to notice an accidental drop on a phone and reach the toast.
const UNDO_MS = 10_000;

/**
 * Executes board commands against `state`: runs the pure command, applies
 * the storage patch, sends the label and title writes, and offers undo when
 * the Plan asks for it. Decides nothing about what an action means.
 *
 * Writes go out independently; a partial failure leaves the other chats
 * where they were and each failed write surfaces a toast. The returned
 * promises settle after that report and never reject.
 */
export const useBoardApi = (
	state: BoardState,
	updateStorage: (patch: Partial<BoardStorage>) => void,
) => {
	const queryClient = useQueryClient();
	const labelsMutation = useMutation({
		...updateChatLabels(queryClient),
		onError: (error: unknown) => {
			toast.error(getErrorMessage(error, "Failed to update chat labels."));
		},
	});
	const titleMutation = useMutation({
		...updateChatTitle(queryClient),
		onError: (error: unknown) => {
			toast.error(getErrorMessage(error, "Failed to rename chat."));
		},
	});

	// onError above has already told the user; a rejection reaching the
	// caller would only add an unhandled-rejection report.
	const reported = (writes: Promise<unknown>): Promise<void> =>
		writes.then(
			() => undefined,
			() => undefined,
		);

	const write = ({ chat, labels }: Write) =>
		labelsMutation.mutateAsync({ chatId: chat.id, labels });

	const run = (plan: Plan | null): Promise<void> => {
		if (!plan) return Promise.resolve();
		if (plan.storage) updateStorage(plan.storage);
		// Snapshot before sending: undo restores the maps as they were, not
		// as the optimistic cache patch left them.
		const before = plan.writes.map(({ chat }) => ({
			chat,
			labels: chat.labels,
		}));
		const pending = [
			...plan.writes.map(write),
			...(plan.titles ?? []).map(({ chat, title }) =>
				titleMutation.mutateAsync({ chatId: chat.id, title }),
			),
		];
		return reported(
			Promise.all(pending).then(() => {
				if (!plan.undo) return;
				toast(plan.undo, {
					duration: UNDO_MS,
					action: {
						label: "Undo",
						onClick: () => void reported(Promise.all(before.map(write))),
					},
				});
			}),
		);
	};

	const bind =
		<A extends unknown[]>(
			command: (state: BoardState, ...args: A) => Plan | null,
		) =>
		(...args: A) =>
			run(command(state, ...args));

	return {
		moveCard: bind(moveCard),
		mergeCards: bind(mergeCards),
		joinCard: bind(joinCard),
		detachChat: bind(detachChat),
		removeFromGroup: bind(removeFromGroup),
		moveColumn: bind(moveColumn),
		addColumn: bind(addColumn),
		renameColumn: bind(renameColumn),
		deleteColumn: bind(deleteColumn),
		setCardColor: bind(setCardColor),
		renameCard: bind(renameCard),
		renameChat: bind(renameChat),
		addNote: bind(addNote),
		editNote: bind(editNote),
		removeNote: bind(removeNote),
		moveNote: bind(moveNote),
	};
};
