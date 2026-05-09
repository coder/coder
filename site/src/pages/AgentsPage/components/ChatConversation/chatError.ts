import type * as TypesGen from "#/api/typesGenerated";
import type { ChatDetailError } from "../../utils/usageLimitMessage";

export const normalizeChatErrorPayload = (
	error: TypesGen.ChatError | undefined,
): ChatDetailError | undefined => {
	const message = error?.message?.trim();
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
		kind: error?.kind ?? "generic",
		provider: error?.provider?.trim() || undefined,
		retryable: error?.retryable,
		statusCode,
		...(detail ? { detail } : {}),
	};
};

/**
 * Image-related error patterns from provider 400 responses.
 * Matches detail/message text from Anthropic, OpenAI, and other
 * providers when an image in the conversation is too large,
 * malformed, or uses an unsupported format.
 */
const IMAGE_ERROR_PATTERNS = [
	"image exceeds",
	"image size",
	"invalid base64",
	"unsupported media type",
	"invalid image",
	"image too large",
	"could not process image",
	"image.source",
] as const;

/**
 * Returns true when the chat error is caused by a problematic image
 * in the conversation (too large, malformed base64, unsupported
 * format, etc.). Used to show targeted guidance about editing or
 * removing the offending image.
 */
export const isImageRelatedError = (
	error: ChatDetailError | undefined | null,
): boolean => {
	if (!error) {
		return false;
	}
	const haystack = [error.message, error.detail]
		.filter(Boolean)
		.join(" ")
		.toLowerCase();
	return IMAGE_ERROR_PATTERNS.some((pattern) => haystack.includes(pattern));
};
