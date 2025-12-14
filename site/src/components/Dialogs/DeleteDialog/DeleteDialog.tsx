import TextField from "@mui/material/TextField";
import { type FC, type FormEvent, useId, useState } from "react";
import { Stack } from "../../Stack/Stack";
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
	const inputColor = hasError ? "error" : "primary";

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
					<Stack spacing={1.5}>
						<p>
							{verb ?? "Deleting"} this {entity} is irreversible!
						</p>

						{Boolean(info) && (
							<div className="border border-solid border-orange-500 bg-orange-950 text-orange-50 rounded-lg py-2 px-4">
								{info}
							</div>
						)}

						<p>
							Type <strong>{name}</strong> below to confirm.
						</p>
					</Stack>

					<form onSubmit={onSubmit}>
						<TextField
							fullWidth
							autoFocus
							className="mt-6"
							name="confirmation"
							autoComplete="off"
							id={`${hookId}-confirm`}
							placeholder={name}
							value={userConfirmationText}
							onChange={(event) => setUserConfirmationText(event.target.value)}
							onFocus={() => setIsFocused(true)}
							onBlur={() => setIsFocused(false)}
							label={label ?? `Name of the ${entity} to delete`}
							color={inputColor}
							error={displayErrorMessage}
							helperText={
								displayErrorMessage &&
								`${userConfirmationText} does not match the name of this ${entity}`
							}
							InputProps={{ color: inputColor }}
							inputProps={{
								"data-testid": "delete-dialog-name-confirmation",
							}}
						/>
					</form>
				</>
			}
		/>
	);
};
