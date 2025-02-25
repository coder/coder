import Link from "@mui/material/Link";
import TextField from "@mui/material/TextField";
import { provisionerDaemons } from "api/queries/organizations";
import type {
	CreateTemplateVersionRequest,
	Organization,
	ProvisionerJobLog,
	ProvisionerType,
	Template,
	TemplateExample,
	TemplateVersionVariable,
	VariableValue,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Button } from "components/Button/Button";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import { IconField } from "components/IconField/IconField";
import { OrganizationAutocomplete } from "components/OrganizationAutocomplete/OrganizationAutocomplete";
import { Spinner } from "components/Spinner/Spinner";
import { useFormik } from "formik";
import camelCase from "lodash/camelCase";
import capitalize from "lodash/capitalize";
import { ProvisionerTagsField } from "modules/provisioners/ProvisionerTagsField";
import { SelectedTemplate } from "pages/CreateWorkspacePage/SelectedTemplate";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router-dom";
import { docs } from "utils/docs";
import {
	displayNameValidator,
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import {
	type TemplateAutostartRequirementDaysValue,
	type TemplateAutostopRequirementDaysValue,
	sortedDays,
} from "utils/schedule";
import * as Yup from "yup";
import { TemplateUpload, type TemplateUploadProps } from "./TemplateUpload";
import { VariableInput } from "./VariableInput";

const MAX_DESCRIPTION_CHAR_LIMIT = 128;

export interface CreateTemplateFormData {
	name: string;
	display_name: string;
	description: string;
	icon: string;
	default_ttl_hours: number;
	autostart_requirement_days_of_week: TemplateAutostartRequirementDaysValue[];
	autostop_requirement_days_of_week: TemplateAutostopRequirementDaysValue;
	autostop_requirement_weeks: number;
	allow_user_autostart: boolean;
	allow_user_autostop: boolean;
	allow_user_cancel_workspace_jobs: boolean;
	parameter_values_by_name?: Record<string, string>;
	user_variable_values?: VariableValue[];
	allow_everyone_group_access: boolean;
	provisioner_type: ProvisionerType;
	organization: string;
	tags: CreateTemplateVersionRequest["tags"];
}

const validationSchema = Yup.object({
	name: nameValidator("Name"),
	display_name: displayNameValidator("Display name"),
	description: Yup.string().max(
		MAX_DESCRIPTION_CHAR_LIMIT,
		"Please enter a description that is less than or equal to 128 characters.",
	),
	icon: Yup.string().optional(),
});

const defaultInitialValues: CreateTemplateFormData = {
	name: "",
	display_name: "",
	description: "",
	icon: "",
	default_ttl_hours: 24,
	// autostop_requirement is an enterprise-only feature, and the server ignores
	// the value if you are not licensed. We hide the form value based on
	// entitlements.
	//
	// Default to requiring restart every Sunday in the user's quiet hours in the
	// user's timezone.
	autostop_requirement_days_of_week: "sunday",
	autostop_requirement_weeks: 1,
	autostart_requirement_days_of_week: sortedDays,
	allow_user_cancel_workspace_jobs: false,
	allow_user_autostart: false,
	allow_user_autostop: false,
	allow_everyone_group_access: true,
	provisioner_type: "terraform",
	organization: "default",
	tags: {},
};

type GetInitialValuesParams = {
	fromExample?: TemplateExample;
	fromCopy?: Template;
	variables?: TemplateVersionVariable[];
	allowAdvancedScheduling: boolean;
	searchParams: URLSearchParams;
};

const getInitialValues = ({
	fromExample,
	fromCopy,
	allowAdvancedScheduling,
	variables,
	searchParams,
}: GetInitialValuesParams) => {
	let initialValues = defaultInitialValues;

	// Will assume the query param has a valid ProvisionerType, as this query param is only used
	// in testing.
	defaultInitialValues.provisioner_type =
		(searchParams.get("provisioner_type") as ProvisionerType) || "terraform";

	if (!allowAdvancedScheduling) {
		initialValues = {
			...initialValues,
			autostop_requirement_days_of_week: "off",
			autostop_requirement_weeks: 1,
		};
	}

	if (fromExample) {
		initialValues = {
			...initialValues,
			name: fromExample.id,
			display_name: fromExample.name,
			icon: fromExample.icon,
			description: fromExample.description,
		};
	}

	if (fromCopy) {
		initialValues = {
			...initialValues,
			...fromCopy,
			name: `${fromCopy.name}-copy`,
			display_name: fromCopy.display_name
				? `Copy of ${fromCopy.display_name}`
				: "",
		};
	}

	if (variables) {
		for (const variable of variables) {
			if (!initialValues.user_variable_values) {
				initialValues.user_variable_values = [];
			}
			initialValues.user_variable_values.push({
				name: variable.name,
				value: variable.sensitive ? "" : variable.value,
			});
		}
	}

	return initialValues;
};

type CopiedTemplateForm = { copiedTemplate: Template };
type StarterTemplateForm = { starterTemplate: TemplateExample };
type UploadTemplateForm = { upload: TemplateUploadProps };

export type CreateTemplateFormProps = (
	| CopiedTemplateForm
	| StarterTemplateForm
	| UploadTemplateForm
) & {
	onCancel: () => void;
	onSubmit: (data: CreateTemplateFormData) => void;
	onOpenBuildLogsDrawer: () => void;
	isSubmitting: boolean;
	variables?: TemplateVersionVariable[];
	error?: unknown;
	jobError?: string;
	logs?: ProvisionerJobLog[];
	allowAdvancedScheduling: boolean;
	variablesSectionRef: React.RefObject<HTMLDivElement>;
	showOrganizationPicker?: boolean;
};

export const CreateTemplateForm: FC<CreateTemplateFormProps> = (props) => {
	const [searchParams] = useSearchParams();
	const [selectedOrg, setSelectedOrg] = useState<Organization | null>(null);
	const {
		onCancel,
		onSubmit,
		onOpenBuildLogsDrawer,
		variables,
		isSubmitting,
		error,
		jobError,
		logs,
		allowAdvancedScheduling,
		variablesSectionRef,
		showOrganizationPicker,
	} = props;

	const form = useFormik<CreateTemplateFormData>({
		initialValues: getInitialValues({
			allowAdvancedScheduling,
			fromExample:
				"starterTemplate" in props ? props.starterTemplate : undefined,
			fromCopy: "copiedTemplate" in props ? props.copiedTemplate : undefined,
			variables,
			searchParams,
		}),
		validationSchema,
		onSubmit,
	});
	const getFieldHelpers = getFormHelpers<CreateTemplateFormData>(form, error);

	const { data: provisioners } = useQuery(
		selectedOrg
			? {
					...provisionerDaemons(selectedOrg.id),
					enabled: showOrganizationPicker,
				}
			: { enabled: false },
	);

	// TODO: Ideally, we would have a backend endpoint that could notify the
	// frontend that a provisioner has been connected, so that we could hide
	// this warning. In the meantime, **do not use this variable to disable
	// form submission**!! A user could easily see this warning, connect a
	// provisioner, and then not refresh the page. Even if they submit without
	// a provisioner, it'll just sit in the job queue until they connect one.
	const showProvisionerWarning = provisioners ? provisioners.length < 1 : false;

	return (
		<HorizontalForm onSubmit={form.handleSubmit}>
			{/* General info */}
			<FormSection
				title="General"
				description="The name is used to identify the template in URLs and the API."
			>
				<FormFields>
					{"starterTemplate" in props && (
						<SelectedTemplate template={props.starterTemplate} />
					)}
					{"upload" in props && (
						<TemplateUpload
							{...props.upload}
							onUpload={async (file) => {
								await fillNameAndDisplayWithFilename(file.name, form);
								props.upload.onUpload(file);
							}}
						/>
					)}

					{showOrganizationPicker && (
						<>
							{showProvisionerWarning && <ProvisionerWarning />}
							<OrganizationAutocomplete
								{...getFieldHelpers("organization")}
								required
								label="Belongs to"
								value={selectedOrg}
								onChange={(newValue) => {
									setSelectedOrg(newValue);
									void form.setFieldValue("organization", newValue?.name || "");
								}}
								size="medium"
								check={{
									object: { resource_type: "template" },
									action: "create",
								}}
							/>
						</>
					)}

					{"copiedTemplate" in props && (
						<SelectedTemplate template={props.copiedTemplate} />
					)}

					<TextField
						{...getFieldHelpers("name")}
						disabled={isSubmitting}
						onChange={onChangeTrimmed(form)}
						fullWidth
						required
						label="Name"
					/>
				</FormFields>
			</FormSection>

			{/* Display info  */}
			<FormSection
				title="Display"
				description="A friendly name, description, and icon to help developers identify your template."
			>
				<FormFields>
					<TextField
						{...getFieldHelpers("display_name")}
						disabled={isSubmitting}
						fullWidth
						label="Display name"
					/>

					<TextField
						{...getFieldHelpers("description", {
							maxLength: MAX_DESCRIPTION_CHAR_LIMIT,
						})}
						disabled={isSubmitting}
						rows={5}
						multiline
						fullWidth
						label="Description"
					/>

					<IconField
						{...getFieldHelpers("icon")}
						disabled={isSubmitting}
						onChange={onChangeTrimmed(form)}
						fullWidth
						onPickEmoji={(value) => form.setFieldValue("icon", value)}
					/>
				</FormFields>
			</FormSection>

			{provisioners && provisioners.length > 0 && (
				<FormSection
					title="Provisioner tags"
					description={
						<>
							Tags are a way to control which provisioner daemons complete which
							build jobs.&nbsp;
							<Link
								href={docs("/admin/provisioners")}
								target="_blank"
								rel="noreferrer"
							>
								Learn more...
							</Link>
						</>
					}
				>
					<FormFields>
						<ProvisionerTagsField
							value={form.values.tags}
							onChange={(tags) => form.setFieldValue("tags", tags)}
						/>
					</FormFields>
				</FormSection>
			)}

			{/* Variables */}
			{variables && variables.length > 0 && (
				<FormSection
					ref={variablesSectionRef}
					title="Variables"
					description="Input variables allow you to customize templates without altering their source code."
				>
					<FormFields>
						{variables.map((variable, index) => (
							<VariableInput
								defaultValue={variable.value}
								variable={variable}
								disabled={isSubmitting}
								key={variable.name}
								onChange={async (value) => {
									await form.setFieldValue(`user_variable_values.${index}`, {
										name: variable.name,
										value,
									});
								}}
							/>
						))}
					</FormFields>
				</FormSection>
			)}

			<FormFooter>
				<Button onClick={onCancel} variant="outline">
					Cancel
				</Button>
				<Button type="submit" disabled={isSubmitting}>
					<Spinner loading={isSubmitting} />
					{jobError ? "Retry" : "Save"}
				</Button>
				{logs && (
					<button
						type="button"
						onClick={onOpenBuildLogsDrawer}
						css={(theme) => ({
							backgroundColor: "transparent",
							border: 0,
							fontWeight: 500,
							fontSize: 14,
							cursor: "pointer",
							color: theme.palette.text.secondary,

							"&:hover": {
								textDecoration: "underline",
								textUnderlineOffset: 4,
								color: theme.palette.text.primary,
							},
						})}
					>
						Show build logs
					</button>
				)}
			</FormFooter>
		</HorizontalForm>
	);
};

const fillNameAndDisplayWithFilename = async (
	filename: string,
	form: ReturnType<typeof useFormik<CreateTemplateFormData>>,
) => {
	const [name, _extension] = filename.split(".");
	await Promise.all([
		form.setFieldValue(
			"name",
			// Camel case will remove special chars and spaces
			camelCase(name).toLowerCase(),
		),
		form.setFieldValue("display_name", capitalize(name)),
	]);
};

const ProvisionerWarning: FC = () => {
	return (
		<Alert severity="warning" css={{ marginBottom: 16 }}>
			This organization does not have any provisioners. Before you create a
			template, you&apos;ll need to configure a provisioner.{" "}
			<Link href={docs("/admin/provisioners#organization-scoped-provisioners")}>
				See our documentation.
			</Link>
		</Alert>
	);
};
