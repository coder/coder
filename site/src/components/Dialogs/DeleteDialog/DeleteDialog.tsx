import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { type FC, type FormEvent, useId, useState } from "react";
import { cn } from "utils/cn";
import { ConfirmDialog } from "../ConfirmDialog/ConfirmDialog";

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
	// All optional to change the verbiage. For example, "unlinking" vs "deleting"
	verb,
	title,
	label,
	confirmText,
}) => {
	const hookId = useId();

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
	const inputId = `${hookId}-confirm`;

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
				<>
					<div className="flex flex-col gap-3">
						<p>
							{verb ?? "Deleting"} this {entity} is irreversible!
						</p>

						{Boolean(info) && (
							<div className="rounded-md border border-solid border-border-destructive bg-surface-destructive px-4 py-2 text-content-destructive">
								{info}
							</div>
						)}

						<p>
							Type <strong>{name}</strong> below to confirm.
						</p>
					</div>

					<form onSubmit={onSubmit} className="mt-6">
						<div className="flex flex-col gap-1">
							<Label htmlFor={inputId}>
								{label ?? `Name of the ${entity} to delete`}
							</Label>
							<Input
								id={inputId}
								name="confirmation"
								autoFocus
								autoComplete="off"
								placeholder={name}
								value={userConfirmationText}
								onChange={(event) =>
									setUserConfirmationText(event.target.value)
								}
								onFocus={() => setIsFocused(true)}
								onBlur={() => setIsFocused(false)}
								aria-invalid={displayErrorMessage}
								className={cn(
									displayErrorMessage &&
										"border-border-destructive focus-visible:ring-border-destructive",
								)}
								data-testid="delete-dialog-name-confirmation"
							/>
							{displayErrorMessage && (
								<p className="text-xs text-content-destructive mt-1">
									{userConfirmationText} does not match the name of this{" "}
									{entity}
								</p>
							)}
						</div>
					</form>
				</>
			}
		/>
	);
};
