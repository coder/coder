import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { updateChatLabels, updateChatTitle } from "#/api/queries/chats";
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

/**
 * Every board action is a set of whole-label-map writes on the chats it
 * touches. Members are written independently; a partial failure leaves the
 * remaining chats where they were and surfaces one toast.
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

	const write = (chat: Chat, labels: Record<string, string>) =>
		labelsMutation.mutateAsync({ chatId: chat.id, labels });

	const moveCard = (card: BoardCard, column: string) =>
		Promise.all(
			card.members.map((member) =>
				write(
					member,
					member.id === card.id
						? setPositionLabel(setColumnLabel(member.labels, column))
						: setColumnLabel(member.labels, column),
				),
			),
		);

	// Renames keep every card where it was; only the column name changes.
	const renameColumn = (cards: readonly BoardCard[], to: string) =>
		Promise.all(
			cards.flatMap((card) =>
				card.members.map((member) =>
					write(member, setColumnLabel(member.labels, to)),
				),
			),
		);

	const setCardTitle = (card: BoardCard, title: string) =>
		write(card.primary, setTitleLabel(card.primary.labels, title));

	const setCardColor = (card: BoardCard, color: CardColor | undefined) =>
		write(card.primary, setColorLabel(card.primary.labels, color));

	const addComment = (card: BoardCard, text: string) =>
		write(card.primary, addCommentLabels(card.primary.labels, text));

	const editComment = (card: BoardCard, index: number, text: string) =>
		write(card.primary, updateCommentLabels(card.primary.labels, index, text));

	const removeComment = (card: BoardCard, index: number) =>
		write(card.primary, removeCommentLabels(card.primary.labels, index));

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
		return Promise.all(writes);
	};

	const detachChat = (chat: Chat, column: string) =>
		write(
			chat,
			setPositionLabel(
				setColumnLabel(setGroupLabel(chat.labels, chat.id, chat.id), column),
			),
		);

	// Only non-primary members are joinable; a primary carries card data that
	// mergeCards handles instead.
	const joinCard = (chat: Chat, target: BoardCard) =>
		write(
			chat,
			setColumnLabel(
				setGroupLabel(chat.labels, target.id, chat.id),
				target.column,
			),
		);

	const renameChat = (chat: Chat, title: string) =>
		titleMutation.mutateAsync({ chatId: chat.id, title });

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
		isPending: labelsMutation.isPending || titleMutation.isPending,
	};
};
