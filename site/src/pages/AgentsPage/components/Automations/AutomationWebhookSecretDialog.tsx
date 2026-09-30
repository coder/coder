import { useRef } from "react";
import { Button } from "#/components/Button/Button";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";

/** Builds the URL that webhook senders post events to. */
export const webhookEventsUrl = (origin: string, automationId: string) =>
	`${origin}/api/experimental/chat-automations/${encodeURIComponent(automationId)}/events`;

type AutomationWebhookSecretDialogProps = {
	endpoint: string;
	secret: string;
	/**
	 * Receives focus on close. The control that led here is often gone, like
	 * the editor's Save button after a create.
	 */
	returnFocusTo?: HTMLElement | null;
	onClose: () => void;
};

/**
 * Shows a new webhook secret once. Callers keep the secret only in component
 * state and drop it on close, so it cannot be shown again.
 */
export const AutomationWebhookSecretDialog: React.FC<
	AutomationWebhookSecretDialogProps
> = ({ endpoint, secret, returnFocusTo, onClose }) => {
	const doneButtonRef = useRef<HTMLButtonElement>(null);
	const curl = `curl -X POST ${endpoint} -H "Authorization: Bearer ${secret}" -H "Content-Type: application/json" -d '{"event":"deploy"}'`;

	return (
		<Dialog
			open
			onOpenChange={(open) => {
				if (!open) {
					onClose();
				}
			}}
		>
			<DialogContent
				// A stray click outside would lose the secret for good.
				onInteractOutside={(event) => event.preventDefault()}
				// Focusing the first copy button would open its tooltip.
				onOpenAutoFocus={(event) => {
					event.preventDefault();
					doneButtonRef.current?.focus();
				}}
				onCloseAutoFocus={(event) => {
					if (returnFocusTo?.isConnected) {
						event.preventDefault();
						returnFocusTo.focus();
					}
				}}
			>
				<DialogHeader>
					<DialogTitle>Copy the webhook secret</DialogTitle>
					<DialogDescription asChild>
						<div className="text-sm text-content-secondary font-medium [&_strong]:text-content-primary [&_p]:m-0 [&_p+p]:mt-2">
							<p>You will not see this secret again.</p>
							<p className="mt-4">Publish endpoint</p>
							<CodeExample
								secret={false}
								code={endpoint}
								copyLabel="Copy endpoint"
								className="mt-1"
							/>
							<p className="mt-4">Secret</p>
							<CodeExample
								secret={false}
								code={secret}
								copyLabel="Copy secret"
								className="mt-1 select-all"
							/>
							<p className="mt-4">Example request</p>
							<CodeExample
								secret={false}
								code={curl}
								copyLabel="Copy example request"
								className="mt-1"
							/>
						</div>
					</DialogDescription>
				</DialogHeader>
				<DialogFooter>
					<Button ref={doneButtonRef} onClick={onClose}>
						Done
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
