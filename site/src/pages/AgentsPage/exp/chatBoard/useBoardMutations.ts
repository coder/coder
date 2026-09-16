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
	nextCommentIndex,
	removeCommentLabels,
	setColorLabel,
	setColumnLabel,
	setGroupLabel,
	setPositionLabel,
	setTitleLabel,
	stripCardLabels,
	updateCommentLabels,
} from "./boardLabels";
import { updateChatLabels } from "./updateChatLabels";

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

	// The dropped card's comments follow its members into the target thread
	// so nothing the user wrote is lost; its title is dropped.
	const mergeCards = (source: BoardCard, target: BoardCard) => {
		const targetLabels = { ...target.primary.labels };
		let index = nextCommentIndex(target.comments);
		for (const comment of source.comments) {
			Object.assign(
				targetLabels,
				commentLabels(index, comment.text, comment.timestamp),
			);
			index += 1;
		}
		const writes = source.members.map((member) =>
			write(
				member,
				setColumnLabel(
					setGroupLabel(stripCardLabels(member.labels), target.id, member.id),
					target.column,
				),
			),
		);
		if (source.comments.length > 0) {
			writes.push(write(target.primary, targetLabels));
		}
		return reported(Promise.all(writes));
	};

	const detachChat = (chat: Chat, column: string, placedAt: number) =>
		reported(
			write(
				chat,
				setPositionLabel(
					setColumnLabel(setGroupLabel(chat.labels, chat.id, chat.id), column),
					placedAt,
				),
			),
		);

	// Only non-primary members are joinable; a primary carries card data that
	// mergeCards handles instead.
	const joinCard = (chat: Chat, target: BoardCard) =>
		reported(
			write(
				chat,
				setColumnLabel(
					setGroupLabel(chat.labels, target.id, chat.id),
					target.column,
				),
			),
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
