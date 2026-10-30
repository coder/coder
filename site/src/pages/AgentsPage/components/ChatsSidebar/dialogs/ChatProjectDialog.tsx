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
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import { getFormHelpers } from "#/utils/formUtils";
import {
	chatProjectDescriptionMaxChars,
	chatProjectFieldsSchema,
	chatProjectIconMaxChars,
	chatProjectNameMaxChars,
	measureLength,
} from "../../chatProjectFields";
import { OptionCheckbox } from "../../ProjectPage/OptionCheckbox";

/** @public */
export type ChatProjectFormValues = {
	/** Set only when creating a project. */
	organizationId?: Organization["id"];
	name: string;
	description: string;
	icon: string;
	/** MCP servers to attach to a new agent. Set only to a non-empty list. */
	mcpServerIds?: string[];
};

/** The form's own values: the MCP servers are always a list, maybe empty. */
type ChatProjectDialogFormValues = Omit<
	ChatProjectFormValues,
	"mcpServerIds"
> & {
	mcpServerIds: string[];
};

/** A built-in MCP server of an organization that a new agent can start with. */
export type QuickAddMCPServer = {
	readonly id: string;
	readonly label: string;
	readonly iconURL: string;
};

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
	/**
	 * Built-in MCP servers each organization has, by organization ID. A new
	 * agent can start with any of them.
	 */
	readonly quickAddMCPServers?: Readonly<
		Record<string, readonly QuickAddMCPServer[]>
	>;
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
	quickAddMCPServers = {},
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
					quickAddMCPServers={quickAddMCPServers}
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
	readonly quickAddMCPServers: NonNullable<
		ChatProjectDialogProps["quickAddMCPServers"]
	>;
	readonly isSubmitting: boolean;
	readonly error: unknown;
	readonly onCancel: () => void;
	readonly onSubmit: ChatProjectDialogProps["onSubmit"];
};

const ChatProjectForm: React.FC<ChatProjectFormProps> = ({
	project,
	organizations,
	initialOrganizationId,
	quickAddMCPServers,
	isSubmitting,
	error,
	onCancel,
	onSubmit,
}) => {
	const form = useFormik<ChatProjectDialogFormValues>({
		initialValues: {
			organizationId: project?.organization_id ?? initialOrganizationId ?? "",
			name: project?.name ?? "",
			description: project?.description ?? "",
			icon: project?.icon ?? "",
			mcpServerIds: [],
		},
		validateOnMount: true,
		validationSchema: project
			? chatProjectFieldsSchema
			: chatProjectFieldsSchema.shape({
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
				...(!project &&
					values.mcpServerIds.length > 0 && {
						mcpServerIds: values.mcpServerIds,
					}),
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
	const quickAddOptions =
		quickAddMCPServers[form.values.organizationId ?? ""] ?? [];
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
				<DialogTitle>{project ? "Edit agent" : "Create an agent"}</DialogTitle>
			</DialogHeader>
			<form className="flex flex-col gap-4" onSubmit={handleSubmit} noValidate>
				<FormField
					field={nameField}
					label={project ? "Name" : "Agent name"}
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
								? "No organizations are available. You need access to an organization to create an agent."
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
								onChange={(organization) => {
									// The servers belong to the previous organization.
									void form.setFieldValue("mcpServerIds", []);
									return form.setFieldValue(
										"organizationId",
										organization?.id ?? "",
									);
								}}
								required
								disabled={isSubmitting}
							/>
						)}
					/>
				)}
				{!project && quickAddOptions.length > 0 && (
					<fieldset className="m-0 flex flex-col gap-2 border-0 p-0">
						<legend className="mb-1 p-0 text-sm font-medium text-content-primary">
							Start with MCP servers
						</legend>
						{quickAddOptions.map((server) => (
							<OptionCheckbox
								key={server.id}
								label={server.label}
								icon={
									server.iconURL ? (
										<ExternalImage
											src={server.iconURL}
											alt=""
											className="size-4 shrink-0 object-contain"
										/>
									) : undefined
								}
								checked={form.values.mcpServerIds.includes(server.id)}
								disabled={isSubmitting}
								onCheckedChange={(checked) =>
									void form.setFieldValue(
										"mcpServerIds",
										checked
											? [...form.values.mcpServerIds, server.id]
											: form.values.mcpServerIds.filter(
													(id) => id !== server.id,
												),
									)
								}
							/>
						))}
					</fieldset>
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
						{project ? "Save" : "Create agent"}
					</Button>
				</DialogFooter>
			</form>
		</>
	);
};
