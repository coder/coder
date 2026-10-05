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
import { type FormHelpers, getFormHelpers } from "#/utils/formUtils";

export type ChatProjectFormValues = {
	name: string;
	description: string;
	icon: string;
};

// Keep in sync with chatProject*MaxChars in coderd/chat_projects.go.
const nameMaxChars = 64;
const descriptionMaxChars = 1024;
const iconMaxChars = 256;

// Counts code points, as the server does. Yup's max() and native maxLength
// count UTF-16 units.
const countCharacters = (value: unknown) => [...String(value ?? "")].length;

const maxCharacters = (label: string, max: number) =>
	Yup.string()
		.trim()
		.test(
			"max-characters",
			`${label} cannot be longer than ${max} characters.`,
			(value) => countCharacters(value) <= max,
		);

const validationSchema = Yup.object({
	name: maxCharacters("Name", nameMaxChars).required("Name is required."),
	description: maxCharacters("Description", descriptionMaxChars),
	icon: maxCharacters("Icon", iconMaxChars),
});

const trimValues = (values: ChatProjectFormValues): ChatProjectFormValues => ({
	name: values.name.trim(),
	description: values.description.trim(),
	icon: values.icon.trim(),
});

// Mirrors the live counter getFormHelpers shows for maxLength, counting code
// points, so an over-limit value is explained before the field loses focus.
const withCharacterCount = (field: FormHelpers, max: number): FormHelpers => {
	const count = countCharacters(field.value);
	if (count <= max - 30) {
		return field;
	}
	const message = `This cannot be longer than ${max} characters. (${count}/${max})`;
	if (count > max) {
		return { ...field, error: true, helperText: message };
	}
	return field.error ? field : { ...field, helperText: message };
};

type ChatProjectDialogProps = {
	readonly project?: ChatProject;
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly isSubmitting: boolean;
	/**
	 * The save error. The caller's mutation outlives the dialog, so reset it
	 * (for example with `mutation.reset()`) before opening.
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
	const nameField = withCharacterCount(getFieldHelpers("name"), nameMaxChars);
	const descriptionField = withCharacterCount(
		getFieldHelpers("description"),
		descriptionMaxChars,
	);
	const iconField = getFieldHelpers("icon");
	const trimmed = trimValues(form.values);
	// An unchanged edit would still bump updated_at and write an audit entry.
	const isUnchanged =
		project !== undefined &&
		trimmed.name === project.name &&
		trimmed.description === project.description &&
		trimmed.icon === project.icon;
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
