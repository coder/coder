import type { Ref } from "react";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";

type ProjectEditConflictAlertProps = {
	/** Explains what changed and what each action does. */
	readonly children: React.ReactNode;
	readonly disabled: boolean;
	/** Keeps the draft and lets Save replace the newer change. */
	readonly onKeepDraft: () => void;
	/** Replaces the draft with the newer change. */
	readonly onLoadLatest: () => void;
	readonly loadLatestButtonRef?: Ref<HTMLButtonElement>;
};

/**
 * Warns that a project setting changed elsewhere while its editor was open,
 * and asks whether to keep the draft or load the latest value.
 */
export const ProjectEditConflictAlert: React.FC<
	ProjectEditConflictAlertProps
> = ({
	children,
	disabled,
	onKeepDraft,
	onLoadLatest,
	loadLatestButtonRef,
}) => (
	<Alert
		severity="warning"
		actions={
			<>
				<Button
					type="button"
					size="sm"
					variant="outline"
					disabled={disabled}
					onClick={onKeepDraft}
				>
					Keep my draft
				</Button>
				<Button
					ref={loadLatestButtonRef}
					type="button"
					size="sm"
					disabled={disabled}
					onClick={onLoadLatest}
				>
					Load latest
				</Button>
			</>
		}
	>
		<AlertDescription>{children}</AlertDescription>
	</Alert>
);
