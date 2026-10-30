import { cn } from "cn";
import { useFormik } from "formik";
import { CheckIcon, PenIcon, XIcon } from "lucide-react";
import { useId, useRef, useState } from "react";
import { getErrorMessageWithValidations } from "#/api/errors";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";

type EditorProps = {
	readonly id: string;
	readonly value: string;
	readonly onChange: (value: string) => void;
	readonly readOnly: boolean;
	readonly invalid: boolean;
	readonly describedBy: string | undefined;
};

type InlineEditFieldProps = {
	/** Names the edit button and the input. */
	readonly label: string;
	readonly value: string;
	/**
	 * Saves the draft. A rejection keeps the field open and shows the error
	 * beside it, so pass the promise of the write, for example from
	 * `mutateAsync`. Omit to show the value without an edit button.
	 */
	readonly onSave?: (value: string) => Promise<unknown>;
	/** Returns a message when the draft cannot be saved. */
	readonly validate?: (value: string) => string | undefined;
	readonly multiline?: boolean;
	/** Shown in place of an empty value. */
	readonly emptyLabel?: string;
	/** Keeps the edit button beside the value instead of at the row's end. */
	readonly fitContent?: boolean;
	/** Saves when focus leaves the field. Turn off for editors with popovers. */
	readonly saveOnBlur?: boolean;
	readonly renderValue?: (value: string) => React.ReactNode;
	readonly renderEditor?: (props: EditorProps) => React.ReactNode;
};

/**
 * Shows a value with an edit button. Editing saves on Enter (Ctrl or Cmd
 * with Enter for multiline text), on the check button, or when focus leaves
 * the field. Escape or the cancel button restores the saved value.
 */
export const InlineEditField: React.FC<InlineEditFieldProps> = ({
	label,
	value,
	onSave,
	validate,
	multiline = false,
	emptyLabel = "None",
	fitContent = false,
	saveOnBlur = true,
	renderValue,
	renderEditor,
}) => {
	const inputId = useId();
	const errorId = useId();
	const [isEditing, setIsEditing] = useState(false);
	const [saveError, setSaveError] = useState<string>();
	const editButtonRef = useRef<HTMLButtonElement>(null);
	// Closing the editor can blur it, which must not save the draft again.
	const isClosingRef = useRef(false);

	const close = () => {
		isClosingRef.current = true;
		setIsEditing(false);
		setSaveError(undefined);
		requestAnimationFrame(() => {
			isClosingRef.current = false;
			editButtonRef.current?.focus();
		});
	};

	const form = useFormik({
		initialValues: { draft: value },
		validate: ({ draft }) => {
			const message = validate?.(draft);
			return message ? { draft: message } : {};
		},
		onSubmit: async ({ draft }) => {
			if (draft === value) {
				close();
				return;
			}
			setSaveError(undefined);
			// Editing only starts when onSave exists. The React Compiler cannot
			// compile optional calls inside try/catch.
			const write = onSave ?? (async () => undefined);
			try {
				await write(draft);
				close();
			} catch (err) {
				setSaveError(getErrorMessageWithValidations(err, "Failed to save."));
			}
		},
	});
	const draft = form.values.draft;
	const isSaving = form.isSubmitting;
	const validationError = form.errors.draft;
	const error = validationError ?? saveError;

	const save = () => {
		if (!isSaving && !isClosingRef.current) {
			void form.submitForm();
		}
	};

	const handleKeyDown = (event: React.KeyboardEvent) => {
		if (event.key === "Escape") {
			event.stopPropagation();
			close();
		} else if (
			event.key === "Enter" &&
			multiline &&
			(event.ctrlKey || event.metaKey)
		) {
			event.preventDefault();
			save();
		}
	};

	if (!isEditing) {
		return (
			<div className="flex min-w-0 items-start gap-2">
				<div
					className={cn(
						"min-w-0 break-words text-sm text-content-primary",
						!fitContent && "flex-1",
					)}
				>
					{value === "" ? (
						<span className="text-content-secondary">{emptyLabel}</span>
					) : renderValue ? (
						renderValue(value)
					) : (
						<span className="whitespace-pre-wrap">{value}</span>
					)}
				</div>
				{onSave && (
					<Button
						ref={editButtonRef}
						type="button"
						size="icon"
						variant="subtle"
						className="-my-1.5 shrink-0"
						aria-label={`Edit ${label}`}
						onClick={() => {
							form.resetForm({ values: { draft: value } });
							setIsEditing(true);
						}}
					>
						<PenIcon />
					</Button>
				)}
			</div>
		);
	}

	const editorProps: EditorProps = {
		id: inputId,
		value: draft,
		onChange: (next) => {
			void form.setFieldValue("draft", next);
			setSaveError(undefined);
		},
		readOnly: isSaving,
		invalid: error !== undefined,
		describedBy: error ? errorId : undefined,
	};

	return (
		<form
			className="flex min-w-0 flex-col gap-1"
			onSubmit={(event) => {
				event.preventDefault();
				save();
			}}
			noValidate
			onKeyDown={handleKeyDown}
			onBlur={(event) => {
				const next = event.relatedTarget;
				if (
					saveOnBlur &&
					!(next instanceof Node && event.currentTarget.contains(next))
				) {
					save();
				}
			}}
		>
			<div className="flex items-start gap-2">
				<div className="min-w-0 flex-1">
					{renderEditor ? (
						renderEditor(editorProps)
					) : multiline ? (
						<Textarea
							id={inputId}
							autoFocus
							rows={4}
							aria-label={label}
							aria-invalid={editorProps.invalid}
							aria-describedby={editorProps.describedBy}
							readOnly={isSaving}
							value={draft}
							onChange={(event) => editorProps.onChange(event.target.value)}
						/>
					) : (
						<Input
							id={inputId}
							autoFocus
							aria-label={label}
							aria-invalid={editorProps.invalid}
							aria-describedby={editorProps.describedBy}
							readOnly={isSaving}
							value={draft}
							onChange={(event) => editorProps.onChange(event.target.value)}
						/>
					)}
				</div>
				<Button
					type="submit"
					size="icon"
					variant="outline"
					aria-label={`Save ${label}`}
					disabled={isSaving || validationError !== undefined}
				>
					{isSaving ? <Spinner loading /> : <CheckIcon />}
				</Button>
				<Button
					type="button"
					size="icon"
					variant="outline"
					aria-label={`Cancel editing ${label}`}
					disabled={isSaving}
					onClick={close}
				>
					<XIcon />
				</Button>
			</div>
			{error && (
				<p
					id={errorId}
					role="alert"
					className="m-0 text-xs text-content-destructive"
				>
					{error}
				</p>
			)}
		</form>
	);
};
