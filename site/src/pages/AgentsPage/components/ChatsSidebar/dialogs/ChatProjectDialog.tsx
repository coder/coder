import { useFormik } from "formik";
import * as Yup from "yup";
import type { ChatProject } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import { getFormHelpers } from "#/utils/formUtils";

export type ChatProjectFormValues = {
	name: string;
	description: string;
	icon: string;
};

// Keep in sync with chatProject*MaxChars in coderd/chat_projects.go.
export const chatProjectNameMaxChars = 64;
export const chatProjectDescriptionMaxChars = 1024;
const chatProjectIconMaxChars = 256;

// Counts code points of the value as submitted, as the server does. Yup's
// max() and native maxLength count UTF-16 units.
const measureLength = (value: string) => [...value.trim()].length;

const maxCharacters = (label: string, max: number) =>
	Yup.string().test(
		"max-characters",
		`${label} cannot be longer than ${max} characters.`,
		(value = "") => measureLength(value) <= max,
	);

const validationSchema = Yup.object({
	name: maxCharacters("Name", chatProjectNameMaxChars)
		.trim()
		.required("Name is required."),
	description: maxCharacters(
		"Description",
		chatProjectDescriptionMaxChars,
	).trim(),
	icon: maxCharacters("Icon", chatProjectIconMaxChars).trim(),
});

const trimValues = (values: ChatProjectFormValues): ChatProjectFormValues => ({
	name: values.name.trim(),
	description: values.description.trim(),
	icon: values.icon.trim(),
});

type ChatProjectDialogProps = {
	readonly project?: ChatProject;
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly isSubmitting: boolean;
	/**
	 * The save error. Call `mutation.reset()` before opening; the mutation
	 * outlives the dialog.
	 */
	readonly error: unknown;
	/** Receives all three fields, trimmed, including unchanged ones on edit. */
	readonly onSubmit: (values: ChatProjectFormValues) => void;
};

export const ChatProjectDialog: React.FC<ChatProjectDialogProps> = ({
	project,
	open,
	onOpenChange,
	isSubmitting,
	error,
	onSubmit,
}) => {
	const handleOpenChange = (nextOpen: boolean) => {
		if (!nextOpen && !isSubmitting) {
			onOpenChange(false);
		}
	};

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			{/* Radix unmounts the content on close, so the form state below
			    resets on every open without remounting the dialog itself. */}
			<DialogContent>
				<ChatProjectForm
					project={project}
					isSubmitting={isSubmitting}
					error={error}
					onCancel={() => handleOpenChange(false)}
					onSubmit={onSubmit}
				/>
			</DialogContent>
		</Dialog>
	);
};

type ChatProjectFormProps = {
	readonly project?: ChatProject;
	readonly isSubmitting: boolean;
	readonly error: unknown;
	readonly onCancel: () => void;
	readonly onSubmit: (values: ChatProjectFormValues) => void;
};

const ChatProjectForm: React.FC<ChatProjectFormProps> = ({
	project,
	isSubmitting,
	error,
	onCancel,
	onSubmit,
}) => {
	const form = useFormik<ChatProjectFormValues>({
		initialValues: {
			name: project?.name ?? "",
			description: project?.description ?? "",
			icon: project?.icon ?? "",
		},
		validateOnMount: true,
		validationSchema,
		onSubmit: (values) => onSubmit(trimValues(values)),
	});
	const getFieldHelpers = getFormHelpers(form, error);
	const nameField = getFieldHelpers("name", {
		maxLength: chatProjectNameMaxChars,
		measureLength,
	});
	const descriptionField = getFieldHelpers("description", {
		maxLength: chatProjectDescriptionMaxChars,
		measureLength,
	});
	const iconField = getFieldHelpers("icon", {
		maxLength: chatProjectIconMaxChars,
		measureLength,
	});
	const trimmed = trimValues(form.values);
	const trimmedInitial = trimValues(form.initialValues);
	// An unchanged edit would still bump updated_at and write an audit entry.
	const isUnchanged =
		project !== undefined &&
		trimmed.name === trimmedInitial.name &&
		trimmed.description === trimmedInitial.description &&
		trimmed.icon === trimmedInitial.icon;
	const canSave = form.isValid && !isUnchanged && !isSubmitting;

	return (
		<>
			<DialogHeader>
				<DialogTitle>{project ? "Edit project" : "New project"}</DialogTitle>
			</DialogHeader>
			<form className="flex flex-col gap-4" onSubmit={form.handleSubmit}>
				<FormField
					field={nameField}
					label="Name"
					required
					disabled={isSubmitting}
					autoFocus
				/>
				<FormField
					field={descriptionField}
					label="Description"
					control={(props) => (
						<Textarea
							{...props}
							name={descriptionField.name}
							value={descriptionField.value}
							onChange={descriptionField.onChange}
							onBlur={descriptionField.onBlur}
							disabled={isSubmitting}
						/>
					)}
				/>
				<IconField
					{...iconField}
					disabled={isSubmitting}
					onPickEmoji={(value) => form.setFieldValue("icon", value)}
				/>
				{Boolean(error) && <ErrorAlert error={error} />}
				<DialogFooter>
					<Button
						type="button"
						variant="outline"
						disabled={isSubmitting}
						onClick={onCancel}
					>
						Cancel
					</Button>
					<Button type="submit" disabled={!canSave}>
						<Spinner loading={isSubmitting} />
						Save
					</Button>
				</DialogFooter>
			</form>
		</>
	);
};
