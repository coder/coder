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
import { restoreFocusTo } from "#/hooks/useRestoreFocusOnClose";

type AutomationWebhookSecretDialogProps = {
	endpoint: string;
	secret: string;
	/** The dialog has no trigger, so Radix cannot restore focus on its own. */
	returnFocusRef: React.RefObject<HTMLElement | null>;
	onClose: () => void;
};

/** Shows a new webhook secret once. */
export const AutomationWebhookSecretDialog: React.FC<
	AutomationWebhookSecretDialogProps
> = ({ endpoint, secret, returnFocusRef, onClose }) => {
	const doneButtonRef = useRef<HTMLButtonElement>(null);

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
				// Only Escape and Done dismiss it; a stray outside click would lose the secret.
				onInteractOutside={(event) => event.preventDefault()}
				// Focusing the first copy button would open its tooltip.
				onOpenAutoFocus={(event) => {
					event.preventDefault();
					doneButtonRef.current?.focus();
				}}
				onCloseAutoFocus={(event) =>
					restoreFocusTo(event, returnFocusRef.current)
				}
			>
				<DialogHeader>
					<DialogTitle>Copy the webhook secret</DialogTitle>
					<DialogDescription>
						You will not see this secret again.
					</DialogDescription>
				</DialogHeader>
				<div className="flex flex-col gap-4">
					{[
						{
							label: "Publish endpoint",
							code: endpoint,
							copyLabel: "Copy endpoint",
						},
						{ label: "Secret", code: secret, copyLabel: "Copy secret" },
						{
							label: "Example request",
							code: `curl -X POST ${endpoint} -H "Authorization: Bearer ${secret}" -H "Content-Type: application/json" -d '{"event":"deploy"}'`,
							copyLabel: "Copy example request",
						},
					].map(({ label, code, copyLabel }) => (
						<div key={label} className="flex flex-col gap-1">
							<span className="text-sm font-medium text-content-primary">
								{label}
							</span>
							<CodeExample secret={false} code={code} copyLabel={copyLabel} />
						</div>
					))}
				</div>
				<DialogFooter>
					<Button ref={doneButtonRef} onClick={onClose}>
						Done
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
