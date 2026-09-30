import { useFormik } from "formik";
import { useEffect, useRef, useState } from "react";
import * as Yup from "yup";
import { getErrorMessage } from "#/api/errors";
import type { ChatProject, Organization } from "#/api/typesGenerated";
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
import { CompactOrgSelector } from "../../ChatElements/CompactOrgSelector";

type ChatProjectFormValues = {
	name: string;
	description: string;
	icon: string;
	organization_id?: string;
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
	/**
	 * Organizations where the user can create a chat. Create sends one of
	 * these ids. The selector is shown only when there is more than one.
	 * Edit ignores this list.
	 */
	readonly organizations?: readonly Organization[];
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly isSubmitting: boolean;
	readonly error: unknown;
	readonly onSubmit: (values: ChatProjectFormValues) => void;
};

export const ChatProjectDialog: React.FC<ChatProjectDialogProps> = ({
	project,
	organizations = [],
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
					organizations={organizations}
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
	readonly organizations: readonly Organization[];
	readonly isSubmitting: boolean;
	readonly error: unknown;
	readonly onCancel: () => void;
	readonly onSubmit: (values: ChatProjectFormValues) => void;
};

const ChatProjectForm: React.FC<ChatProjectFormProps> = ({
	project,
	organizations,
	isSubmitting,
	error,
	onCancel,
	onSubmit,
}) => {
	const [selectedOrganizationId, setSelectedOrganizationId] = useState<
		string | undefined
	>(undefined);
	const selectedOrganization = project
		? undefined
		: (organizations.find(
				(organization) => organization.id === selectedOrganizationId,
			) ??
			organizations.find((organization) => organization.is_default) ??
			organizations[0]);
	const showOrganizationSelector = !project && organizations.length > 1;
	const selectedOrganizationRef = useRef(selectedOrganization);
	useEffect(() => {
		selectedOrganizationRef.current = selectedOrganization;
	}, [selectedOrganization]);
	const form = useFormik<ChatProjectFormValues>({
		initialValues: {
			name: project?.name ?? "",
			description: project?.description ?? "",
			icon: project?.icon ?? "",
		},
		validateOnMount: true,
		validationSchema,
		onSubmit: (values) => {
			const name = values.name.trim();
			const description = values.description.trim();
			const icon = values.icon.trim();
			if (project) {
				onSubmit({ name, description, icon });
				return;
			}
			const organization = selectedOrganizationRef.current;
			if (!organization) {
				return;
			}
			onSubmit({
				name,
				description,
				icon,
				organization_id: organization.id,
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
				{showOrganizationSelector && (
					<CompactOrgSelector
						value={selectedOrganization ?? null}
						options={organizations}
						disabled={isSubmitting}
						onChange={(organization) =>
							setSelectedOrganizationId(organization.id)
						}
					/>
				)}
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
					<Button
						type="submit"
						disabled={
							!form.isValid ||
							isSubmitting ||
							(!project && !selectedOrganization)
						}
					>
						<Spinner loading={isSubmitting} />
						Save
					</Button>
				</DialogFooter>
			</form>
		</>
	);
};
