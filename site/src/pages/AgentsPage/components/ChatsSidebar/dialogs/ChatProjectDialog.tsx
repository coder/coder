import { useFormik } from "formik";
import * as Yup from "yup";
import { getErrorMessage } from "#/api/errors";
import type { ChatProject } from "#/api/typesGenerated";
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

type ChatProjectFormValues = {
	name: string;
	description: string;
	icon: string;
};

const nameMaxLength = 64;
const descriptionMaxLength = 1024;
const iconMaxLength = 256;

const validationSchema = Yup.object({
	name: Yup.string()
		.trim()
		.required("Name is required.")
		.max(
			nameMaxLength,
			`Name cannot be longer than ${nameMaxLength} characters.`,
		),
	description: Yup.string().max(
		descriptionMaxLength,
		`Description cannot be longer than ${descriptionMaxLength} characters.`,
	),
	icon: Yup.string().max(
		iconMaxLength,
		`Icon cannot be longer than ${iconMaxLength} characters.`,
	),
});

type ChatProjectDialogProps = {
	readonly project?: ChatProject;
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly isSubmitting: boolean;
	readonly error: unknown;
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
	const getFieldHelpers = getFormHelpers(form);
	const nameField = getFieldHelpers("name", { maxLength: nameMaxLength });
	const descriptionField = getFieldHelpers("description", {
		maxLength: descriptionMaxLength,
	});
	const iconField = getFieldHelpers("icon", { maxLength: iconMaxLength });

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
					maxLength={nameMaxLength}
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
							maxLength={descriptionMaxLength}
						/>
					)}
				/>
				<IconField
					{...iconField}
					disabled={isSubmitting}
					maxLength={iconMaxLength}
					onPickEmoji={(value) => form.setFieldValue("icon", value)}
				/>
				{Boolean(error) && (
					<p className="m-0 text-sm text-content-destructive">
						{getErrorMessage(error, "Failed to save project.")}
					</p>
				)}
				<DialogFooter>
					<Button
						type="button"
						variant="outline"
						disabled={isSubmitting}
						onClick={onCancel}
					>
						Cancel
					</Button>
					<Button type="submit" disabled={!form.isValid || isSubmitting}>
						<Spinner loading={isSubmitting} />
						Save
					</Button>
				</DialogFooter>
			</form>
		</>
	);
};
