import { useEffect, useEffectEvent, useState } from "react";
import { useQueryClient } from "react-query";
import { invalidateChatDiffContents } from "#/api/queries/chats";
import type { ChatDiffReauthRequired } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { toast } from "#/components/Toaster/toast";

const POPUP_POLL_INTERVAL_MS = 500;

type ChatDiffReauthNoticeProps = {
	chatId: string;
	reauth: ChatDiffReauthRequired;
};

/**
 * Replaces the diff when the provider rejected the chat owner's token. The
 * owner reconnects in a popup, and the diff reloads once the popup closes.
 * Other viewers see a message asking the owner to reconnect.
 */
export const ChatDiffReauthNotice: React.FC<ChatDiffReauthNoticeProps> = ({
	chatId,
	reauth,
}) => {
	const {
		provider_id: providerId,
		provider_display_name: providerName,
		can_reconnect: canReconnect,
	} = reauth;
	const queryClient = useQueryClient();
	// The open popup doubles as the connecting state, so the two cannot drift.
	const [popup, setPopup] = useState<Window | null>(null);
	const isConnecting = popup !== null;

	const onPopupClosed = useEffectEvent(() => {
		setPopup(null);
		void invalidateChatDiffContents(queryClient, chatId);
	});

	useEffect(() => {
		if (!popup) {
			return;
		}
		const timer = window.setInterval(() => {
			if (popup.closed) {
				onPopupClosed();
			}
		}, POPUP_POLL_INTERVAL_MS);
		return () => window.clearInterval(timer);
	}, [popup]);

	const reconnect = () => {
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
			<p className="m-0 text-sm text-content-secondary">
				{canReconnect
					? `Reconnect ${providerName} to load this diff.`
					: `The chat owner must reconnect ${providerName} to load this diff.`}
			</p>
			{canReconnect && (
				<Button
					size="sm"
					variant="outline"
					disabled={isConnecting}
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
