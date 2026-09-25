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
 * State for a "type the name to confirm" field. The owning dialog uses
 * `confirmed` to enable its delete action and calls `reset` when it closes or
 * confirms, so reopening never shows a previously typed name.
 */
export const useDeleteConfirmation = (name: string) => {
	const [value, setValue] = useState("");
	const [isFocused, setIsFocused] = useState(false);
	const [hasSubmittedInvalid, setHasSubmittedInvalid] = useState(false);

	const confirmed = value === name;
	const showError =
		!confirmed && value.length > 0 && (!isFocused || hasSubmittedInvalid);

	return {
		name,
		value,
		confirmed,
		showError,
		reset: () => {
			setValue("");
			setIsFocused(false);
			setHasSubmittedInvalid(false);
		},
		inputProps: {
			value,
			onChange: (event: ChangeEvent<HTMLInputElement>) => {
				setValue(event.target.value);
				setHasSubmittedInvalid(false);
			},
			onFocus: () => setIsFocused(true),
			onBlur: () => setIsFocused(false),
			onKeyDown: (event: KeyboardEvent<HTMLInputElement>) => {
				// Handled here rather than in onSubmit because forms whose submit
				// button is disabled for a wrong name never submit, while forms
				// without a submit button would submit a wrong name.
				if (event.key === "Enter" && !confirmed) {
					event.preventDefault();
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
			{/*
			 * Always rendered at a fixed one-line height so showing the error never
			 * moves the controls below it, for example when pressing Cancel blurs
			 * the input. Visually only the typed value truncates, keeping its
			 * whitespace so a stray space stays visible inside the quotes. The split
			 * layout would add a space to the announced text, so assistive tech gets
			 * the message as a single string instead.
			 */}
			<p
				id={errorId}
				role="alert"
				className="m-0 h-4 text-xs leading-4 text-content-destructive"
			>
				{showError && (
					<>
						<span className="sr-only">
							&ldquo;{value}&rdquo; does not match the name of this {entity}
						</span>
						<span aria-hidden className="flex min-w-0 overflow-hidden">
							<span className="min-w-0 overflow-hidden text-ellipsis whitespace-pre">
								&ldquo;{value}
							</span>
							<span className="shrink-0 whitespace-nowrap">
								&rdquo; does not match the name of this {entity}
							</span>
						</span>
					</>
				)}
			</p>
		</div>
	);
};
