import { type FC, type FormEvent, useId, useState } from "react";
import { Alert } from "#/components/Alert/Alert";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { ConfirmDialog } from "./ConfirmDialog";

interface DeleteDialogProps {
	isOpen: boolean;
	onConfirm: () => void;
	onCancel: () => void;
	entity: string;
	name: string;
	info?: string;
	confirmLoading?: boolean;
	verb?: string;
	title?: string;
	label?: string;
	confirmText?: string;
}

export const DeleteDialog: FC<DeleteDialogProps> = ({
	isOpen,
	onCancel,
	onConfirm,
	entity,
	info,
	name,
	confirmLoading,
	// All optional to change the verbiage. For example, "unlinking" vs "deleting".
	verb,
	title,
	label,
	confirmText,
}) => {
	const inputId = useId();
	const errorId = useId();

	const [userConfirmationText, setUserConfirmationText] = useState("");
	const [isFocused, setIsFocused] = useState(false);

	const deletionConfirmed = name === userConfirmationText;
	const onSubmit = (event: FormEvent) => {
		event.preventDefault();
		if (deletionConfirmed) {
			onConfirm();
		}
	};

	const hasError = !deletionConfirmed && userConfirmationText.length > 0;
	const displayErrorMessage = hasError && !isFocused;
	const inputLabel = label ?? `Name of the ${entity} to delete`;

	return (
		<ConfirmDialog
			type="delete"
			hideCancel={false}
			open={isOpen}
			title={title ?? `Delete ${entity}`}
			onConfirm={onConfirm}
			onClose={onCancel}
			confirmLoading={confirmLoading}
			disabled={!deletionConfirmed}
			confirmText={confirmText}
			description={
				<div className="flex flex-col gap-6">
					<div className="flex flex-col gap-3">
						<p className="m-0">
							{verb ?? "Deleting"} this {entity} is irreversible!
						</p>
						{Boolean(info) && (
							<Alert severity="warning" prominent>
								{info}
							</Alert>
						)}
						<p className="m-0">
							Type <strong className="text-content-primary">{name}</strong>{" "}
							below to confirm.
						</p>
					</div>

					<form onSubmit={onSubmit} className="flex flex-col gap-2">
						<Label htmlFor={inputId} className="sr-only">
							{inputLabel}
						</Label>
						<Input
							autoFocus
							id={inputId}
							name="confirmation"
							autoComplete="off"
							placeholder={name}
							value={userConfirmationText}
							onChange={(event) => setUserConfirmationText(event.target.value)}
							onFocus={() => setIsFocused(true)}
							onBlur={() => setIsFocused(false)}
							aria-label={inputLabel}
							aria-invalid={displayErrorMessage}
							aria-describedby={displayErrorMessage ? errorId : undefined}
							data-testid="delete-dialog-name-confirmation"
						/>
						{displayErrorMessage && (
							<p id={errorId} className="m-0 text-xs text-content-destructive">
								{userConfirmationText} does not match the name of this {entity}
							</p>
						)}
					</form>
				</div>
			}
		/>
	);
};
