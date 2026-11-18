import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "react-query";
import { invalidateChatDiffContents } from "#/api/queries/chats";
import type { ChatDiffReauthRequired } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { toast } from "#/components/Toaster/toast";

type ChatDiffReauthNoticeProps = {
	chatId: string;
	reauth: ChatDiffReauthRequired;
};

/**
 * Explains that the provider rejected the chat owner's token. The owner can
 * reconnect inline; the diff reloads once the reconnect popup closes.
 */
export const ChatDiffReauthNotice: React.FC<ChatDiffReauthNoticeProps> = ({
	chatId,
	reauth,
}) => {
	const queryClient = useQueryClient();
	const popupRef = useRef<Window | null>(null);
	const [isConnecting, setIsConnecting] = useState(false);

	useEffect(() => {
		if (!isConnecting) {
			return;
		}
		const interval = setInterval(() => {
			if (!popupRef.current?.closed) {
				return;
			}
			popupRef.current = null;
			setIsConnecting(false);
			void invalidateChatDiffContents(queryClient, chatId);
		}, 500);
		return () => clearInterval(interval);
	}, [isConnecting, chatId, queryClient]);

	const reconnect = () => {
		const popup = window.open(
			`/external-auth/${encodeURIComponent(reauth.provider_id)}`,
			"_blank",
			"width=900,height=600",
		);
		if (!popup) {
			toast.error("Pop-up blocked. Allow pop-ups to reconnect.");
			return;
		}
		popupRef.current = popup;
		setIsConnecting(true);
	};

	return (
		<div className="flex h-full flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
			<p className="m-0 text-sm text-content-secondary">
				{reauth.can_reconnect
					? `Reconnect ${reauth.provider_display_name} to load this diff.`
					: `The chat owner must reconnect ${reauth.provider_display_name} to load this diff.`}
			</p>
			{reauth.can_reconnect && (
				<Button
					size="sm"
					variant="outline"
					disabled={isConnecting}
					onClick={reconnect}
				>
					{isConnecting
						? "Waiting for reconnect…"
						: `Reconnect ${reauth.provider_display_name}`}
				</Button>
			)}
		</div>
	);
};
