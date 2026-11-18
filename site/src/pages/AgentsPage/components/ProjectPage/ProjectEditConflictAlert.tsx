import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";

type ProjectEditConflictAlertProps = {
	/** Explains what changed and what each action does. */
	readonly children: React.ReactNode;
	readonly disabled: boolean;
	readonly onKeepDraft: () => void;
	readonly onLoadLatest: () => void;
	readonly loadLatestButtonRef?: React.Ref<HTMLButtonElement>;
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
