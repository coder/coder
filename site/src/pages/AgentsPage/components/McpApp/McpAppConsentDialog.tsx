import type { FC } from "react";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";

export type McpAppConsentRequest =
	| { kind: "message"; text: string }
	| { kind: "open-link"; url: string };

interface McpAppConsentDialogProps {
	request: McpAppConsentRequest | null;
	serverName: string;
	confirmLoading: boolean;
	onConfirm: () => void;
	onDecline: () => void;
}

/** Asks the user before an app sends a chat message or opens a link. */
export const McpAppConsentDialog: FC<McpAppConsentDialogProps> = ({
	request,
	serverName,
	confirmLoading,
	onConfirm,
	onDecline,
}) => {
	const isLink = request?.kind === "open-link";
	return (
		<ConfirmDialog
			open={request !== null}
			type="info"
			hideCancel={false}
			title={
				isLink
					? `Open link from ${serverName}?`
					: `Send message from ${serverName}?`
			}
			description={
				<div className="flex flex-col gap-2">
					<p className="m-0">
						{isLink
							? `The ${serverName} app wants to open this link in a new tab:`
							: `The ${serverName} app wants to send this message to the chat as you:`}
					</p>
					<pre className="m-0 max-h-48 overflow-auto whitespace-pre-wrap break-all rounded-md bg-surface-secondary p-2 text-xs">
						{request?.kind === "open-link" ? request.url : request?.text}
					</pre>
				</div>
			}
			confirmText={isLink ? "Open link" : "Send"}
			confirmLoading={confirmLoading}
			onClose={onDecline}
			onConfirm={onConfirm}
		/>
	);
};
