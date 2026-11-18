import { useEffect, useState } from "react";
import { useQueryClient } from "react-query";
import { invalidateChatDiffContents } from "#/api/queries/chats";
import type { ChatDiffReauthRequired } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { toast } from "#/components/Toaster/toast";

export const POPUP_POLL_INTERVAL_MS = 500;

type ChatDiffReauthNoticeProps = {
	chatId: string;
	reauth: ChatDiffReauthRequired;
};

/**
 * Shown in place of the diff for a ChatDiffReauthRequired error. The owner
 * reconnects in a popup, and the diff refetches when the popup closes, whether
 * or not the reconnect succeeded.
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
	const [popup, setPopup] = useState<Window | null>(null);
	const isConnecting = popup !== null;

	useEffect(() => {
		if (!popup) {
			return;
		}
		const timer = window.setInterval(() => {
			if (popup.closed) {
				setPopup(null);
				void invalidateChatDiffContents(queryClient, chatId);
			}
		}, POPUP_POLL_INTERVAL_MS);
		return () => window.clearInterval(timer);
	}, [popup, queryClient, chatId]);

	const reconnect = () => {
		// The button stays focusable while connecting, so clicks can still arrive.
		if (isConnecting) {
			return;
		}
		const opened = window.open(
			`/external-auth/${encodeURIComponent(providerId)}`,
			"_blank",
			"width=900,height=600",
		);
		if (!opened) {
			toast.error("Pop-up blocked. Allow pop-ups to reconnect.");
			return;
		}
		setPopup(opened);
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
