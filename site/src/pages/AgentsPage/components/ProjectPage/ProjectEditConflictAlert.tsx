import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";

type ProjectEditConflictAlertProps = {
	readonly children: React.ReactNode;
	readonly disabled: boolean;
	readonly onKeepDraft: () => void;
	readonly onLoadLatest: () => void;
	readonly loadLatestButtonRef?: React.Ref<HTMLButtonElement>;
};

/**
 * Each editor resolves a conflict its own way, so `children` must say what
 * keeping the draft does there: replace the newer value, or merge into it.
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
