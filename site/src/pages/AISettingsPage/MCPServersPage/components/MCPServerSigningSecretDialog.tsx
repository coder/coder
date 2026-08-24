import { type FC, useState } from "react";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";

interface MCPServerSigningSecretDialogProps {
	secret: string;
	onClose: () => void;
}

export const MCPServerSigningSecretDialog: FC<
	MCPServerSigningSecretDialogProps
> = ({ secret, onClose }) => {
	// Remount after rejected dismiss attempts so the one-time secret stays visible until Done.
	const [dismissAttempt, setDismissAttempt] = useState(0);

	return (
		<Dialog
			key={dismissAttempt}
			open={secret !== ""}
			onOpenChange={(open) => {
				if (!open) {
					setDismissAttempt((attempt) => attempt + 1);
				}
			}}
		>
			<DialogContent aria-describedby={undefined}>
				<DialogHeader>
					<DialogTitle>Save your MCP signing secret</DialogTitle>
				</DialogHeader>
				<div className="flex flex-col gap-5">
					<Alert severity="warning">
						<AlertDescription>
							Copy this secret now. For security reasons it cannot be shown
							again.
						</AlertDescription>
					</Alert>
					<CodeExample
						secret={false}
						code={secret}
						className="min-h-0 select-all w-full"
					/>
					<DialogFooter>
						<Button onClick={onClose}>Done</Button>
					</DialogFooter>
				</div>
			</DialogContent>
		</Dialog>
	);
};
