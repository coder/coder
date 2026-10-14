import { cn } from "cn";
import { useFormik } from "formik";
import { useState } from "react";
import * as Yup from "yup";
import { isApiValidationError } from "#/api/errors";
import type { ChatProject, Organization } from "#/api/typesGenerated";
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
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import { getFormHelpers } from "#/utils/formUtils";

/** @public */
export type ChatProjectFormValues = {
	/** Set only when creating a project. */
	organizationId?: Organization["id"];
	name: string;
	description: string;
	icon: string;
};

// Keep in sync with chatProject*MaxChars in coderd/chat_projects.go.
const chatProjectNameMaxChars = 64;
const chatProjectDescriptionMaxChars = 1024;
const chatProjectIconMaxChars = 256;

// Counts code points of the value as submitted, as the server does. Yup's
// max() and native maxLength count UTF-16 units.
const measureLength = (value: string) => [...value.trim()].length;

const maxCharacters = (label: string, max: number) =>
	Yup.string()
		.trim()
		.test(
			"max-characters",
			`${label} cannot be longer than ${max} characters.`,
			(value = "") => measureLength(value) <= max,
		);

const validationSchema = Yup.object({
	name: maxCharacters("Name", chatProjectNameMaxChars).required(
		"Name is required.",
	),
	description: maxCharacters("Description", chatProjectDescriptionMaxChars),
	icon: maxCharacters("Icon", chatProjectIconMaxChars),
});

const trimValues = (values: ChatProjectFormValues): ChatProjectFormValues => ({
	name: values.name.trim(),
	description: values.description.trim(),
	icon: values.icon.trim(),
});

const isUnchangedEdit = (
	project: ChatProject | undefined,
	values: ChatProjectFormValues,
	initial: ChatProjectFormValues,
) => {
	if (project === undefined) {
		return false;
	}
	const trimmed = trimValues(values);
	const trimmedInitial = trimValues(initial);
	return (
		trimmed.name === trimmedInitial.name &&
		trimmed.description === trimmedInitial.description &&
		trimmed.icon === trimmedInitial.icon
	);
};

type ChatProjectDialogProps = {
	readonly project?: ChatProject;
	readonly organizations?: readonly Organization[];
	readonly initialOrganizationId?: Organization["id"];
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly isSubmitting: boolean;
	/**
	 * The save error. Call `mutation.reset()` before opening; the mutation
	 * outlives the dialog.
	 */
	readonly error: unknown;
	/**
	 * Receives the name, description, and icon, trimmed, including unchanged
	 * ones on edit. Also receives the selected organization when creating.
	 * Return the save's promise (for example from `mutateAsync`) so Save stays
	 * disabled until it settles. A rejection is ignored here; report it
	 * through `error`.
	 */
	readonly onSubmit: (values: ChatProjectFormValues) => unknown;
};

export const ChatProjectDialog: React.FC<ChatProjectDialogProps> = ({
	project,
	organizations = [],
	initialOrganizationId,
	open,
	onOpenChange,
	isSubmitting,
	error,
	onSubmit,
}) => {
	// Covers the window between Save and the caller's isSubmitting update.
	const [isSaving, setIsSaving] = useState(false);
	const isPending = isSubmitting || isSaving;
	const handleOpenChange = (nextOpen: boolean) => {
		if (!nextOpen && !isPending) {
			onOpenChange(false);
		}
	};
	const handleSubmit = (values: ChatProjectFormValues) => {
		setIsSaving(true);
		return Promise.resolve()
			.then(() => onSubmit(values))
			.finally(() => setIsSaving(false));
	};

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent aria-describedby={undefined}>
				<ChatProjectForm
					project={project}
					organizations={organizations}
					initialOrganizationId={initialOrganizationId}
					isSubmitting={isPending}
					error={error}
					onCancel={() => handleOpenChange(false)}
					onSubmit={handleSubmit}
				/>
			</DialogContent>
		</Dialog>
	);
};

type ChatProjectFormProps = {
	readonly project?: ChatProject;
	readonly organizations: readonly Organization[];
	readonly initialOrganizationId?: Organization["id"];
	readonly isSubmitting: boolean;
	readonly error: unknown;
	readonly onCancel: () => void;
	readonly onSubmit: ChatProjectDialogProps["onSubmit"];
};

const ChatProjectForm: React.FC<ChatProjectFormProps> = ({
	project,
	organizations,
	initialOrganizationId,
	isSubmitting,
	error,
	onCancel,
	onSubmit,
}) => {
	const form = useFormik<ChatProjectFormValues>({
		initialValues: {
			organizationId: project?.organization_id ?? initialOrganizationId ?? "",
			name: project?.name ?? "",
			description: project?.description ?? "",
			icon: project?.icon ?? "",
		},
		validateOnMount: true,
		validationSchema: project
			? validationSchema
			: validationSchema.shape({
					organizationId: Yup.string()
						.required("Organization is required.")
						.oneOf(
							organizations.map((organization) => organization.id),
							"Select an available organization.",
						),
				}),
		// Formik keeps isSubmitting set until this settles, so a second click
		// cannot submit again before the caller's isSubmitting turns on.
		onSubmit: async (values) => {
			// Built outside the try block: the React Compiler cannot compile
			// conditional expressions inside try/catch.
			const submitted = {
				...(!project && { organizationId: values.organizationId }),
				...trimValues(values),
			};
			try {
				await onSubmit(submitted);
			} catch {
				// The caller shows the failure through the error prop.
			}
		},
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
	const organizationField = getFieldHelpers("organizationId");
	const selectedOrganization = organizations.find(
		(organization) => organization.id === form.values.organizationId,
	);
	const isUnchanged = isUnchangedEdit(project, form.values, form.initialValues);
	const isSaving = isSubmitting || form.isSubmitting;
	const canCreate = form.dirty && selectedOrganization !== undefined;
	const canSave =
		form.isValid &&
		!isUnchanged &&
		!isSaving &&
		(project !== undefined || canCreate);
	const handleSubmit = (event: React.FormEvent<HTMLFormElement>) => {
		if (isSaving || isUnchanged) {
			event.preventDefault();
			return;
		}
		form.handleSubmit(event);
	};

	return (
		<>
			<DialogHeader>
				<DialogTitle>
					{project ? "Edit project" : "Create a project"}
				</DialogTitle>
			</DialogHeader>
			<form className="flex flex-col gap-4" onSubmit={handleSubmit} noValidate>
				<FormField
					field={nameField}
					label={project ? "Name" : "Project name"}
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
							{...form.getFieldProps("description")}
							disabled={isSubmitting}
							rows={3}
							className={cn(
								descriptionField.error && "border-border-destructive",
							)}
						/>
					)}
				/>
				{!project && (
					<FormField
						field={organizationField}
						label="Organization"
						required
						description={
							organizations.length === 0
								? "No organizations are available. You need access to an organization to create a project."
								: !selectedOrganization && form.values.organizationId
									? "The selected organization is no longer available. Select another organization."
									: undefined
						}
						control={(props) => (
							<OrganizationAutocomplete
								{...props}
								ariaLabel={
									selectedOrganization
										? `Organization ${getOrganizationLabel(selectedOrganization, organizations)}`
										: "Organization: Select an organization…"
								}
								value={selectedOrganization ?? null}
								options={organizations}
								onChange={(organization) =>
									form.setFieldValue("organizationId", organization?.id ?? "")
								}
								required
								disabled={isSubmitting}
							/>
						)}
					/>
				)}
				<IconField
					{...iconField}
					disabled={isSubmitting}
					onPickEmoji={(value) => {
						void form.setFieldValue("icon", value);
					}}
				/>
				{Boolean(error) && !isApiValidationError(error) && (
					<ErrorAlert error={error} showDebugDetail={false} />
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
					<Button type="submit" disabled={!canSave}>
						<Spinner loading={isSaving} />
						{project ? "Save" : "Create project"}
					</Button>
				</DialogFooter>
			</form>
		</>
	);
};
