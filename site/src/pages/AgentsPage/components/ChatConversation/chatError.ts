import type * as TypesGen from "#/api/typesGenerated";
import type { ChatDetailError } from "../../utils/usageLimitMessage";

type StructuredChatError = TypesGen.ChatLastError | TypesGen.ChatStreamError;

interface NormalizeChatErrorOptions {
	fallbackMessage?: string;
}

export const normalizeChatErrorPayload = (
	error: StructuredChatError | undefined,
	options?: NormalizeChatErrorOptions,
): ChatDetailError | undefined => {
	const fallbackMessage = options?.fallbackMessage?.trim();
	const message = error?.message?.trim() || fallbackMessage;
	if (!message) {
		return undefined;
	}
	const detail = error?.detail?.trim();
	const statusCode =
		typeof error?.status_code === "number" && error.status_code > 0
			? error.status_code
			: undefined;
	return {
		message,
		kind: error?.kind?.trim() || "generic",
		provider: error?.provider?.trim() || undefined,
		retryable: error?.retryable,
		statusCode,
		...(detail ? { detail } : {}),
	};
};
