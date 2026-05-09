import { TimerIcon, UserIcon } from "lucide-react";
import { useId, useState } from "react";
import { Alert } from "#/components/Alert/Alert";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Dialog,
	DialogActions,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "./Dialog";
import { type DateTimeInput, relativeTime } from "#/utils/time";

type DeleteDialogProps = {
	open: boolean;
	description: React.ReactNode;
	requireAcknowledgingName?: boolean;
	additionalInfo?: React.ReactNode;

	resourceKind: string;
	resourceName: string;
	resourceLastUsed?: DateTimeInput;
	resourceOwnedBy?: string;

	deleteAction: React.ReactNode;
	deleteLoading?: boolean;
	onDelete: () => void;

	onCancel: () => void;
};

export const DeleteDialog: React.FC<DeleteDialogProps> = ({
	open,
	description,
	requireAcknowledgingName,
	additionalInfo,

	resourceKind,
	resourceName,
	resourceLastUsed,
	resourceOwnedBy,

	deleteAction = "Delete",
	deleteLoading,
	onDelete,

	onCancel,
}) => {
	const [acknowledged, setAcknowledged] = useState(false);

	const includeSubtitle = resourceLastUsed || resourceOwnedBy;

	return (
		<Dialog
			open={open}
			onOpenChange={(nextOpen) => {
				if (!nextOpen) {
					onCancel();
				}
			}}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>
						{deleteAction} {resourceName}?
					</DialogTitle>
					{includeSubtitle && (
						<div className="flex flex-row gap-3 text-sm">
							{resourceLastUsed && (
								<div className="flex flex-row items-center gap-1">
									<TimerIcon className="size-4" />
									<span>last used {relativeTime(resourceLastUsed)}</span>
								</div>
							)}
							{resourceOwnedBy && (
								<div className="flex flex-row items-center gap-1">
									<UserIcon className="size-4" />
									<span>owned by {resourceOwnedBy}</span>
								</div>
							)}
						</div>
					)}
				</DialogHeader>
				<form action={onDelete} className="flex flex-col gap-6 [&_*]:m-0">
					<div className="flex flex-col gap-3">
						<DialogDescription>{description}</DialogDescription>
						{Boolean(additionalInfo) && (
							<Alert severity="warning" prominent>
								{additionalInfo}
							</Alert>
						)}
					</div>
					{requireAcknowledgingName && (
						<DeleteDialogAcknowledgmentInput
							resourceName={resourceName}
							resourceKind={resourceKind}
							setAcknowledged={setAcknowledged}
						/>
					)}
					<DialogFooter>
						<DialogActions
							confirmAction={deleteAction}
							confirmLoading={deleteLoading}
							confirmDisabled={requireAcknowledgingName && !acknowledged}
							confirmVariant="destructive"
							onConfirm={onDelete}
							onCancel={onCancel}
						/>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
};

type DeleteDialogAcknowledgmentInputProps = {
	resourceName: string;
	resourceKind: string;
	setAcknowledged: (acknowledged: boolean) => void;
};

const DeleteDialogAcknowledgmentInput: React.FC<
	DeleteDialogAcknowledgmentInputProps
> = ({ resourceName, resourceKind, setAcknowledged }) => {
	const inputId = useId();
	const errorId = useId();
	const [showError, setShowError] = useState(false);

	return (
		<div className="flex flex-col gap-3">
			<Label htmlFor={inputId}>Confirm the name of the {resourceKind}</Label>
			<Input
				id={inputId}
				onChange={(event) => {
					const inputValue = event.target.value;
					setAcknowledged(inputValue === resourceName);
				}}
				onFocus={() => setShowError(false)}
				onBlur={(event) => {
					const inputValue = event.target.value;
					setShowError(inputValue.length > 0 && inputValue !== resourceName);
				}}
				aria-invalid={showError}
				aria-describedby={showError ? errorId : undefined}
				autoFocus
			/>
			{showError && (
				<Alert severity="error">
					Please enter the name of the {resourceKind}
				</Alert>
			)}
		</div>
	);
};
