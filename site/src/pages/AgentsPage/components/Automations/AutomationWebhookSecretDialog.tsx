import { useState } from "react";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";

/** Builds the URL that webhook senders post events to. */
export const webhookEventsUrl = (origin: string, automationId: string) =>
	`${origin}/api/experimental/chat-automations/${encodeURIComponent(automationId)}/events`;

type AutomationWebhookSecretDialogProps = {
	endpoint: string;
	secret: string;
	onClose: () => void;
};

/**
 * Shows a new webhook secret once. Callers keep the secret only in component
 * state and drop it on close, so it cannot be shown again.
 */
export const AutomationWebhookSecretDialog: React.FC<
	AutomationWebhookSecretDialogProps
> = ({ endpoint, secret, onClose }) => {
	// Radix returns focus to a DialogTrigger on close; this dialog has none.
	const [opener] = useState(() =>
		document.activeElement instanceof HTMLElement
			? document.activeElement
			: null,
	);
	const curl = `curl -X POST ${endpoint} -H "Authorization: Bearer ${secret}" -H "Content-Type: application/json" -d '{"event":"deploy"}'`;

	return (
		<ConfirmDialog
			open
			type="info"
			title="Copy the webhook secret"
			confirmText="Done"
			onClose={onClose}
			onCloseAutoFocus={(event) => {
				if (opener?.isConnected) {
					event.preventDefault();
					opener.focus();
				}
			}}
			description={
				<>
					<p>You will not see this secret again.</p>
					<p className="mt-4">Publish endpoint</p>
					<CodeExample secret={false} code={endpoint} className="mt-1" />
					<p className="mt-4">Secret</p>
					<CodeExample
						secret={false}
						code={secret}
						className="mt-1 select-all"
					/>
					<p className="mt-4">Example request</p>
					<CodeExample secret={false} code={curl} className="mt-1" />
				</>
			}
		/>
	);
};
