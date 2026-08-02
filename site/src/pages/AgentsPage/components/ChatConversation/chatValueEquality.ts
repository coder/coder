import type * as TypesGen from "#/api/typesGenerated";

const jsonValuesEqual = (left: unknown, right: unknown): boolean => {
	if (left === right) {
		return true;
	}
	try {
		return JSON.stringify(left) === JSON.stringify(right);
	} catch {
		return false;
	}
};

export const chatMessagesEqualByValue = (
	left: TypesGen.ChatMessage,
	right: TypesGen.ChatMessage,
): boolean =>
	left.id === right.id &&
	left.chat_id === right.chat_id &&
	left.model_config_id === right.model_config_id &&
	left.created_at === right.created_at &&
	left.role === right.role &&
	jsonValuesEqual(left.content, right.content) &&
	jsonValuesEqual(left.usage, right.usage);

// Compares two queue snapshots by value. Deliberately not by ID alone: a
// requeued edit keeps the ID and changes the content. Covers every field of
// `ChatQueuedMessage`, so it agrees with the structural sharing the query
// cache applies to a write.
export const chatQueuedMessagesEqualByValue = (
	left: readonly TypesGen.ChatQueuedMessage[],
	right: readonly TypesGen.ChatQueuedMessage[],
): boolean =>
	left.length === right.length &&
	left.every((message, index) => {
		const other = right[index];
		return (
			message.id === other.id &&
			message.chat_id === other.chat_id &&
			message.model_config_id === other.model_config_id &&
			message.created_at === other.created_at &&
			jsonValuesEqual(message.content, other.content)
		);
	});
