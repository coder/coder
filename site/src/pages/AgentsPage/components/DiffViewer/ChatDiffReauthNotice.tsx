import { useEffect, useEffectEvent, useState } from "react";
import { useQuery, useQueryClient } from "react-query";
import { invalidateChatDiffContents } from "#/api/queries/chats";
import { externalAuthProvider } from "#/api/queries/externalAuth";
import type { ChatDiffReauthRequired } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { toast } from "#/components/Toaster/toast";

export const POPUP_POLL_INTERVAL_MS = 500;
// Each status poll validates the token with the provider, so poll less often
// than the popup check, and stop polling after a minute.
export const POPUP_STATUS_POLL_INTERVAL_MS = 2000;
export const POPUP_STATUS_POLL_TIMEOUT_MS = 60_000;
const POPUP_FEATURES = "width=900,height=600";

type Connection = {
	popup: Window;
	openedAt: number;
};

type ChatDiffReauthNoticeProps = {
	chatId: string;
	reauth: ChatDiffReauthRequired;
};

/**
 * Shown in place of the diff for a ChatDiffReauthRequired error. The owner
 * reconnects in a popup. The diff refetches when the popup closes, whether or
 * not the reconnect succeeded, and also once the provider reports the account
 * authenticated.
 */
export const ChatDiffReauthNotice: React.FC<ChatDiffReauthNoticeProps> = ({
	chatId,
	reauth,
}) => {
	const {
		message,
		provider_id: providerId,
		provider_display_name: providerName,
		can_reconnect: canReconnect,
	} = reauth;
	const queryClient = useQueryClient();
	const [connection, setConnection] = useState<Connection | null>(null);
	const isConnecting = connection !== null;
	const openedAt = connection?.openedAt ?? 0;

	// Connecting ends when the popup closes, or when the provider reports the
	// account authenticated after the popup opened.
	const { data: externalAuth, dataUpdatedAt } = useQuery({
		...externalAuthProvider(providerId),
		enabled: isConnecting,
		refetchInterval: () =>
			Date.now() - openedAt < POPUP_STATUS_POLL_TIMEOUT_MS
				? POPUP_STATUS_POLL_INTERVAL_MS
				: false,
	});

	const onConnectEnded = useEffectEvent(() => {
		setConnection(null);
		void invalidateChatDiffContents(queryClient, chatId);
	});

	const onConnectPoll = useEffectEvent(() => {
		if (connection?.popup.closed) {
			onConnectEnded();
			return;
		}
		// Cached status from before the popup opened can say authenticated for an
		// account that is no longer connected, so count only fresh status.
		if (externalAuth?.authenticated && dataUpdatedAt >= openedAt) {
			onConnectEnded();
		}
	});

	useEffect(() => {
		if (!connection) {
			return;
		}
		const timer = window.setInterval(onConnectPoll, POPUP_POLL_INTERVAL_MS);
		return () => window.clearInterval(timer);
	}, [connection]);

	const reconnect = () => {
		// The button stays focusable while connecting, so clicks can still arrive.
		if (isConnecting) {
			return;
		}
		const popup = window.open(
			`/external-auth/${encodeURIComponent(providerId)}`,
			"_blank",
			POPUP_FEATURES,
		);
		if (!popup) {
			toast.error("Pop-up blocked. Allow pop-ups to reconnect.");
			return;
		}
		setConnection({ popup, openedAt: Date.now() });
	};

	return (
		<div className="flex h-full flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
			<p className="m-0 text-sm text-content-secondary">{message}</p>
			{canReconnect && (
				<Button
					size="sm"
					variant="outline"
					aria-disabled={isConnecting}
					onClick={reconnect}
				>
					{isConnecting
						? "Waiting for reconnect…"
						: `Reconnect ${providerName}`}
				</Button>
			)}
		</div>
	);
};
