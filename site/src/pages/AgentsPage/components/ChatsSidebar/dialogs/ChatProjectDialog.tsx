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

const nameMaxLength = 64;
const descriptionMaxLength = 1024;
const iconMaxLength = 256;

// The server counts characters as code points, so an emoji counts once here
// too. Yup's max() and the native maxLength attribute count UTF-16 units.
const maxCharacters = (label: string, max: number) =>
	Yup.string().test(
		"max-characters",
		`${label} cannot be longer than ${max} characters.`,
		(value = "") => [...value].length <= max,
	);

const validationSchema = Yup.object({
	name: maxCharacters("Name", nameMaxLength)
		.trim()
		.required("Name is required."),
	description: maxCharacters("Description", descriptionMaxLength),
	icon: maxCharacters("Icon", iconMaxLength),
});

type ChatProjectDialogProps = {
	readonly project?: ChatProject;
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly isSubmitting: boolean;
	readonly error: unknown;
	/**
	 * Receives every field, trimmed. Editing sends all three, so it replaces
	 * the stored name, description, and icon.
	 */
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
		onSubmit: (values) => {
			onSubmit({
				name: values.name.trim(),
				description: values.description.trim(),
				icon: values.icon.trim(),
			});
		},
	});
	const getFieldHelpers = getFormHelpers(form, error);
	const nameField = getFieldHelpers("name");
	const descriptionField = getFieldHelpers("description");
	const iconField = getFieldHelpers("icon");
	// An unchanged edit would still bump updated_at and write an audit entry.
	const canSave = form.isValid && (!project || form.dirty) && !isSubmitting;

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
