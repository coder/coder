import { getErrorDetail, getErrorMessage, isApiError } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import {
	type ChatDetailError,
	formatUsageLimitMessage,
	isChatUsageLimitExceededResponse,
} from "../../utils/usageLimitMessage";

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

const defaultChatSendErrorMessage = "Failed to send chat message.";

export const normalizeChatSendError = (
	error: unknown,
): ChatDetailError | undefined => {
	if (!isApiError(error)) {
		return undefined;
	}
	if (
		error.response.status === 409 &&
		isChatUsageLimitExceededResponse(error.response.data)
	) {
		return {
			kind: "usage_limit",
			message: formatUsageLimitMessage(error.response.data),
		};
	}

	const message =
		getErrorMessage(error, defaultChatSendErrorMessage).trim() ||
		defaultChatSendErrorMessage;
	const detail = getErrorDetail(error)?.trim();
	const statusCode =
		error.response.status > 0 ? error.response.status : undefined;
	return {
		kind: "generic",
		message,
		...(detail ? { detail } : {}),
		...(statusCode ? { statusCode } : {}),
	};
};
