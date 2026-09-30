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

type AutomationWebhookSecretDialogProps = {
	endpoint: string;
	secret: string;
	/** Receives focus on close, since the opener may no longer exist. */
	returnFocusTo: HTMLElement | null;
	onClose: () => void;
};

/** Shows a new webhook secret once. */
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
					<DialogDescription>
						You will not see this secret again.
					</DialogDescription>
				</DialogHeader>
				<div className="flex flex-col gap-4">
					{[
						{
							label: "Publish endpoint",
							code: endpoint,
							copy: "Copy endpoint",
						},
						{ label: "Secret", code: secret, copy: "Copy secret" },
						{
							label: "Example request",
							code: curl,
							copy: "Copy example request",
						},
					].map(({ label, code, copy }) => (
						<div key={label} className="flex flex-col gap-1">
							<span className="text-sm font-medium text-content-primary">
								{label}
							</span>
							<CodeExample secret={false} code={code} copyLabel={copy} />
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
