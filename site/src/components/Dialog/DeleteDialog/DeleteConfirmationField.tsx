import {
	type ChangeEvent,
	type FC,
	type KeyboardEvent,
	type ReactNode,
	useId,
	useState,
} from "react";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";

/**
 * State for a "type the name to confirm" field. It clears when `isOpen` becomes
 * false and is kept while the dialog stays open, so a failed delete can be
 * retried.
 */
export const useDeleteConfirmation = (name: string, isOpen: boolean) => {
	const [value, setValue] = useState("");
	const [isFocused, setIsFocused] = useState(false);
	const [hasSubmittedInvalid, setHasSubmittedInvalid] = useState(false);
	const [prevIsOpen, setPrevIsOpen] = useState(isOpen);

	if (isOpen !== prevIsOpen) {
		setPrevIsOpen(isOpen);
		if (!isOpen) {
			setValue("");
			setIsFocused(false);
			setHasSubmittedInvalid(false);
		}
	}

	const confirmed = value === name;
	const showError =
		!confirmed && value.length > 0 && (!isFocused || hasSubmittedInvalid);

	return {
		name,
		value,
		confirmed,
		showError,
		inputProps: {
			value,
			onChange: (event: ChangeEvent<HTMLInputElement>) => {
				setValue(event.target.value);
				setHasSubmittedInvalid(false);
			},
			onFocus: () => setIsFocused(true),
			onBlur: () => setIsFocused(false),
			onKeyDown: (event: KeyboardEvent<HTMLInputElement>) => {
				// DeleteDialog disables its submit button for a wrong name, so its form
				// never submits and onSubmit cannot set this.
				if (event.key === "Enter" && !confirmed) {
					setHasSubmittedInvalid(true);
				}
			},
		},
	};
};

type DeleteConfirmationFieldProps = {
	confirmation: ReturnType<typeof useDeleteConfirmation>;
	label: ReactNode;
	entity: string;
};

export const DeleteConfirmationField: FC<DeleteConfirmationFieldProps> = ({
	confirmation,
	label,
	entity,
}) => {
	const inputId = useId();
	const errorId = `${inputId}-error`;
	const { name, value, showError, inputProps } = confirmation;

	return (
		<div className="flex flex-col gap-2">
			<Label htmlFor={inputId}>{label}</Label>
			<Input
				{...inputProps}
				id={inputId}
				className="text-content-primary"
				name="confirmation"
				autoComplete="off"
				autoFocus
				placeholder={name}
				aria-invalid={showError}
				aria-describedby={showError ? errorId : undefined}
				data-testid="delete-dialog-name-confirmation"
			/>
			{showError && (
				<span
					id={errorId}
					role="alert"
					className="whitespace-pre-wrap text-xs text-content-destructive"
				>
					&ldquo;{value}&rdquo; does not match the name of this {entity}
				</span>
			)}
		</div>
	);
};
