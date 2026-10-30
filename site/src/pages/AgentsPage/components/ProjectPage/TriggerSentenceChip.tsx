import { cn } from "cn";
import { type FormikErrors, type FormikValues, useFormik } from "formik";
import { ChevronDownIcon } from "lucide-react";
import { useState } from "react";
import type {
	ChatProjectTrigger,
	UpdateChatProjectTriggerRequest,
} from "#/api/typesGenerated";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";

/** Lets an editor keep its draft when the save fails. */
export type TriggerUpdateCallbacks = {
	readonly onSuccess?: () => void;
	/** Receives the message already shown to the person. */
	readonly onError?: (message: string) => void;
};

export type TriggerChipProps = {
	readonly trigger: ChatProjectTrigger;
	readonly readOnly: boolean;
	readonly onUpdate: (
		request: UpdateChatProjectTriggerRequest,
		callbacks?: TriggerUpdateCallbacks,
	) => void;
};

type ChipFaceProps = {
	readonly label: string;
	/** Adds the chevron and hover styles of a chip that opens an editor. */
	readonly interactive: boolean;
};

const ChipFace: React.FC<ChipFaceProps> = ({ label, interactive }) => (
	<span
		className={cn(
			"inline-flex min-w-0 items-center gap-1 rounded-md bg-surface-secondary px-2 text-sm font-medium text-content-primary",
			interactive
				? "h-7 max-w-64 transition-colors group-hover:bg-surface-tertiary group-data-[state=open]:bg-surface-tertiary"
				: // Read-only chips have no editor to show the rest of the value.
					"min-h-7 py-0.5 wrap-anywhere",
		)}
	>
		<span className={cn(interactive && "truncate")}>{label}</span>
		{interactive && (
			<ChevronDownIcon
				aria-hidden
				className="size-icon-xs shrink-0 text-content-secondary"
			/>
		)}
	</span>
);

type SentenceChipProps = {
	/** The chip text, such as "Any repository". */
	readonly label: string;
	/** Prefixes the accessible name, such as "Repository". */
	readonly fieldLabel: string;
	readonly readOnly: boolean;
	/** Called when the editor opens, to seed drafts from the saved values. */
	readonly onOpen?: () => void;
	/** Saves drafts when the editor closes. */
	readonly onSave?: () => void;
	/** Called instead of `onSave` when Escape closes the editor. */
	readonly onDiscard?: () => void;
	readonly contentClassName?: string;
	/** The popover editor. Call `close` to save and close it. */
	readonly children: (close: () => void) => React.ReactNode;
};

/** A filled pill in a trigger sentence that opens an editor for its value. */
export const SentenceChip: React.FC<SentenceChipProps> = ({
	label,
	fieldLabel,
	readOnly,
	onOpen,
	onSave,
	onDiscard,
	contentClassName,
	children,
}) => {
	const [open, setOpen] = useState(false);
	if (readOnly) {
		return <ChipFace label={label} interactive={false} />;
	}
	const handleOpenChange = (next: boolean) => {
		setOpen(next);
		if (next) {
			onOpen?.();
		} else {
			onSave?.();
		}
	};
	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<button
					type="button"
					aria-label={`${fieldLabel}: ${label}`}
					className="group inline-flex min-w-0 cursor-pointer rounded-md border-0 bg-transparent p-0 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
				>
					<ChipFace label={label} interactive />
				</button>
			</PopoverTrigger>
			<PopoverContent
				align="start"
				className={cn("flex w-72 flex-col gap-3 p-3", contentClassName)}
				onEscapeKeyDown={(event) => {
					event.preventDefault();
					setOpen(false);
					onDiscard?.();
				}}
			>
				{children(() => handleOpenChange(false))}
			</PopoverContent>
		</Popover>
	);
};

type TriggerChipFormOptions<Values extends FormikValues> = {
	/** The saved values, which a new draft starts from. */
	readonly saved: Values;
	readonly validate?: (values: Values) => FormikErrors<Values>;
	/** The changed fields to save. Empty when nothing changed. */
	readonly toRequest: (values: Values) => UpdateChatProjectTriggerRequest;
	readonly onUpdate: TriggerChipProps["onUpdate"];
};

/**
 * The draft of a chip editor. Closing the editor saves it and Escape drops
 * it. A draft that fails to save or is invalid stays for the next time the
 * editor opens, with the reason in `saveError` or `form.errors`.
 */
export const useTriggerChipForm = <Values extends FormikValues>({
	saved,
	validate,
	toRequest,
	onUpdate,
}: TriggerChipFormOptions<Values>) => {
	const [saveError, setSaveError] = useState<string>();
	const form = useFormik<Values>({
		initialValues: saved,
		validate,
		onSubmit: (values, helpers) => {
			const request = toRequest(values);
			if (Object.keys(request).length === 0) {
				helpers.resetForm({ values: saved });
				return;
			}
			setSaveError(undefined);
			onUpdate(request, {
				// The saved values become the new baseline, so the next
				// opening starts from them.
				onSuccess: () => helpers.resetForm({ values }),
				onError: setSaveError,
			});
		},
	});
	return {
		form,
		saveError,
		chipProps: {
			onOpen: () => {
				if (!form.dirty) {
					form.resetForm({ values: saved });
				}
			},
			onSave: () => void form.submitForm(),
			onDiscard: () => {
				setSaveError(undefined);
				form.resetForm({ values: saved });
			},
		},
	};
};
