import { isAxiosError } from "axios";
import type { ChatDiffReauthRequired } from "#/api/typesGenerated";

// Matches codersdk.ChatDiffReauthRequiredCode on the server.
const REAUTH_REQUIRED_CODE = "external_auth_reauth_required";

/**
 * Returns the reconnect details when a chat diff request failed because the
 * provider rejected the chat owner's token, or undefined for any other error.
 */
export const getChatDiffReauthRequired = (
	error: unknown,
): ChatDiffReauthRequired | undefined => {
	if (!isAxiosError(error) || error.response?.status !== 424) {
		return undefined;
	}
	const data = error.response.data as Partial<ChatDiffReauthRequired> | "";
	if (!data || data.code !== REAUTH_REQUIRED_CODE) {
		return undefined;
	}
	return data as ChatDiffReauthRequired;
};
