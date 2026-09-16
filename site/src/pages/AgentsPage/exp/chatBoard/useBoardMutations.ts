import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { updateChatTitle } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import {
	addCommentLabels,
	type BoardCard,
	type CardColor,
	commentLabels,
	getTitleLabel,
	nextCommentIndex,
	placementKey,
	removeCommentLabels,
	setColorLabel,
	setColumnLabel,
	setGroupLabel,
	setPositionLabel,
	setTitleLabel,
	stripCardLabels,
	takeCardLabels,
	updateCommentLabels,
} from "./boardLabels";
import { updateChatLabels } from "./updateChatLabels";

// Long enough to notice an accidental drop on a phone and reach the toast.
const UNDO_MS = 10_000;

type Write = { readonly chat: Chat; readonly labels: Record<string, string> };

/**
 * Every board action is a set of whole-label-map writes on the chats it
 * touches. Members are written independently; a partial failure leaves the
 * remaining chats where they were, and each failed write surfaces a toast.
 * The returned promises settle after that report and never reject, so
 * callers can fire and forget.
 */
export const useBoardMutations = () => {
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

	const write = (chat: Chat, labels: Record<string, string>) =>
		labelsMutation.mutateAsync({ chatId: chat.id, labels });

	// Regrouping is easy to do by accident with a drop, so every such change
	// offers to put the touched chats' label maps back exactly as they were.
	const commitUndoable = (writes: readonly Write[], done: string) => {
		const before = writes.map(({ chat }) => ({ chat, labels: chat.labels }));
		return reported(
			Promise.all(writes.map(({ chat, labels }) => write(chat, labels))).then(
				() => {
					toast(done, {
						duration: UNDO_MS,
						action: {
							label: "Undo",
							onClick: () =>
								void reported(
									Promise.all(
										before.map(({ chat, labels }) => write(chat, labels)),
									),
								),
						},
					});
				},
			),
		);
	};

	/** Moves a card to a column at a given placement key (see keyBetween). */
	const moveCard = (card: BoardCard, column: string, placedAt: number) =>
		reported(
			Promise.all(
				card.members.map((member) =>
					write(
						member,
						member.id === card.id
							? setPositionLabel(
									setColumnLabel(member.labels, column),
									placedAt,
								)
							: setColumnLabel(member.labels, column),
					),
				),
			),
		);

	// Renames keep every card where it was; only the column name changes.
	const renameColumn = (cards: readonly BoardCard[], to: string) =>
		reported(
			Promise.all(
				cards.flatMap((card) =>
					card.members.map((member) =>
						write(member, setColumnLabel(member.labels, to)),
					),
				),
			),
		);

	const setCardTitle = (card: BoardCard, title: string) =>
		reported(write(card.primary, setTitleLabel(card.primary.labels, title)));

	const setCardColor = (card: BoardCard, color: CardColor | undefined) =>
		reported(write(card.primary, setColorLabel(card.primary.labels, color)));

	const addComment = (card: BoardCard, text: string) =>
		reported(write(card.primary, addCommentLabels(card.primary.labels, text)));

	const editComment = (card: BoardCard, index: number, text: string) =>
		reported(
			write(
				card.primary,
				updateCommentLabels(card.primary.labels, index, text),
			),
		);

	const removeComment = (card: BoardCard, index: number) =>
		reported(
			write(card.primary, removeCommentLabels(card.primary.labels, index)),
		);

	// The side that keeps its primary: an existing group beats a single chat,
	// then a titled card beats an untitled one, then the drop target. So a
	// group dropped onto a lone chat absorbs it instead of losing its title.
	const mergeKeeper = (source: BoardCard, target: BoardCard): BoardCard => {
		const grouped = (card: BoardCard) => card.members.length > 1;
		if (grouped(source) !== grouped(target)) {
			return grouped(source) ? source : target;
		}
		const titled = (card: BoardCard) =>
			getTitleLabel(card.primary) !== undefined;
		if (titled(source) !== titled(target)) {
			return titled(source) ? source : target;
		}
		return target;
	};

	// The kept primary moves into the drop target's slot and absorbs the
	// other card's notes; a title that would otherwise vanish becomes a note.
	const mergeCards = (source: BoardCard, target: BoardCard) => {
		const keep = mergeKeeper(source, target);
		const join = keep === source ? target : source;
		let labels = setPositionLabel(
			setColumnLabel(keep.primary.labels, target.column),
			placementKey(target.primary),
		);
		if (!keep.color && join.color) labels = setColorLabel(labels, join.color);
		let index = nextCommentIndex(keep.comments);
		const carried = join.comments.map((c) => [c.text, c.timestamp] as const);
		if (getTitleLabel(join.primary) && getTitleLabel(keep.primary)) {
			carried.push([`Merged card: ${join.title}`, Date.now()]);
		}
		for (const [text, timestamp] of carried) {
			Object.assign(labels, commentLabels(index, text, timestamp));
			index += 1;
		}
		const writes: Write[] = [
			{ chat: keep.primary, labels },
			...join.members.map((member) => ({
				chat: member,
				labels: setColumnLabel(
					setGroupLabel(stripCardLabels(member.labels), keep.id, member.id),
					target.column,
				),
			})),
			...(keep === target
				? []
				: keep.members
						.filter((member) => member.id !== keep.id)
						.map((member) => ({
							chat: member,
							labels: setColumnLabel(member.labels, target.column),
						}))),
		];
		return commitUndoable(writes, `Merged into "${keep.title}"`);
	};

	// A primary that leaves hands the card (title, color, position, notes) to
	// the oldest remaining member and the others follow it. The card keeps
	// its effective title, so the departure renames nothing.
	const leaveGroup = (chat: Chat, card: BoardCard): Write[] => {
		if (chat.id !== card.id || card.members.length < 2) return [];
		const [next, ...rest] = card.members.filter((m) => m.id !== chat.id);
		if (!next) return [];
		return [
			{
				chat: next,
				labels: setTitleLabel(
					{
						...setGroupLabel(next.labels, next.id, next.id),
						...takeCardLabels(card.primary.labels),
					},
					card.title,
				),
			},
			...rest.map((member) => ({
				chat: member,
				labels: setGroupLabel(member.labels, next.id, member.id),
			})),
		];
	};

	/** Makes the chat its own card in `column` at `placedAt`, whatever its role in `card`. */
	const detachChat = (
		chat: Chat,
		card: BoardCard,
		column: string,
		placedAt: number,
	) =>
		commitUndoable(
			[
				...leaveGroup(chat, card),
				{
					chat,
					labels: setPositionLabel(
						setColumnLabel(stripCardLabels(chat.labels), column),
						placedAt,
					),
				},
			],
			`Removed "${chat.title}" from "${card.title}"`,
		);

	/** Moves one chat out of `card` into `target`. */
	const joinCard = (chat: Chat, card: BoardCard, target: BoardCard) =>
		commitUndoable(
			[
				...leaveGroup(chat, card),
				{
					chat,
					labels: setColumnLabel(
						setGroupLabel(stripCardLabels(chat.labels), target.id, chat.id),
						target.column,
					),
				},
			],
			`Added "${chat.title}" to "${target.title}"`,
		);

	const renameChat = (chat: Chat, title: string) =>
		reported(titleMutation.mutateAsync({ chatId: chat.id, title }));

	return {
		moveCard,
		renameColumn,
		setCardTitle,
		setCardColor,
		addComment,
		editComment,
		removeComment,
		mergeCards,
		detachChat,
		joinCard,
		renameChat,
	};
};
