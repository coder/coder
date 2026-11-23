import { cn } from "cn";
import { useFormik } from "formik";
import { useEffect } from "react";
import { useQuery } from "react-query";
import * as Yup from "yup";
import { getErrorMessage, isApiError } from "#/api/errors";
import {
	permittedOrganizations,
	provisionerDaemons,
} from "#/api/queries/organizations";
import type { Organization } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import { Link } from "#/components/Link/Link";
import { OrganizationAutocomplete } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { RetryButton } from "#/components/RetryButton/RetryButton";
import { Textarea } from "#/components/Textarea/Textarea";
import {
	TemplateBuilderSubtitle,
	TemplateBuilderTitle,
} from "#/pages/TemplateBuilder/TemplateBuilderHeader";
import { docs } from "#/utils/docs";
import {
	displayNameValidator,
	getFormHelpers,
	iconValidator,
	nameValidator,
} from "#/utils/formUtils";
import { StepFrame } from "./StepFrame";
import type {
	CustomizationsFormValues,
	SelectedBaseMeta,
	TemplateBuilderWizardState,
} from "./wizardState";

const MAX_DESCRIPTION_CHAR_LIMIT = 128;

const emptyOrgs: readonly Organization[] = [];

// The create endpoint returns these statuses without field validations, so map
// each one to the field the user has to change.
const createErrorFields: Partial<
	Record<number, keyof CustomizationsFormValues>
> = {
	404: "organization_id",
	409: "name",
};

const validationSchema = Yup.object({
	name: nameValidator("Template ID"),
	display_name: displayNameValidator("Display name"),
	description: Yup.string().max(
		MAX_DESCRIPTION_CHAR_LIMIT,
		"Please enter a description that is less than or equal to 128 characters.",
	),
	icon: iconValidator,
	// An organization is always required: the page is gated on the create-
	// template permission, so there is always at least one permitted org, and
	// it is auto-selected when only one is available.
	organization_id: Yup.string().required("Select an organization to continue."),
});

type TemplateCustomizationsStepProps = {
	state: TemplateBuilderWizardState;
	onCreate: (values: CustomizationsFormValues) => void;
	onBack: () => void;
	/**
	 * Replaces the defaults derived from `state`. Formik reads it only on
	 * mount, which works because the wizard remounts this step after a failed
	 * create.
	 */
	initialValues?: CustomizationsFormValues;
	/** The last create failure, shown on the field it belongs to. */
	createError?: Error | null;
};

export const TemplateCustomizationsStep: React.FC<
	TemplateCustomizationsStepProps
> = ({ state, onCreate, onBack, initialValues, createError }) => {
	const permittedOrgsQuery = useQuery(
		permittedOrganizations({
			object: { resource_type: "template" },
			action: "create",
		}),
	);
	const orgOptions = permittedOrgsQuery.data ?? emptyOrgs;
	const showOrgsError = permittedOrgsQuery.isError && !permittedOrgsQuery.data;

	const form = useFormik<CustomizationsFormValues>({
		initialValues: initialValues ?? {
			organization_id: "",
			name: state.name,
			display_name: state.displayName,
			description: state.description,
			icon: state.icon,
		},
		validationSchema,
		onSubmit: (values) => onCreate(values),
	});
	// A create error sits on the field it names. When it names no field, or
	// that field is not rendered (the Organization field while organizations
	// fail to load), the alert above the form shows it instead.
	const namedCreateErrorField = isApiError(createError)
		? createErrorFields[createError.response.status]
		: undefined;
	const createErrorField =
		namedCreateErrorField === "organization_id" && showOrgsError
			? undefined
			: namedCreateErrorField;
	const showCreateErrorAlert = createError != null && !createErrorField;
	const getFormFieldHelpers = getFormHelpers(form);
	const getFieldHelpers: typeof getFormFieldHelpers = (field, options) => {
		const helpers = getFormFieldHelpers(field, options);
		// Keep the create error on its field until the user changes the value.
		if (
			field === createErrorField &&
			form.values[field] === form.initialValues[field]
		) {
			return {
				...helpers,
				error: true,
				helperText: getErrorMessage(
					createError,
					"Unable to create the template.",
				),
			};
		}
		return helpers;
	};
	const displayNameField = getFieldHelpers("display_name");
	const nameField = getFieldHelpers("name", {
		helperText: "Used to identify the template in URLs and the API.",
	});
	const descriptionField = getFieldHelpers("description");
	const iconField = getFieldHelpers("icon");
	const organizationField = getFieldHelpers("organization_id", {
		helperText: permittedOrgsQuery.isPending
			? "Loading organizations…"
			: undefined,
	});

	const organizationId = form.values.organization_id;
	const selectedOrg = orgOptions.find((o) => o.id === organizationId) ?? null;

	const { data: provisioners } = useQuery({
		...provisionerDaemons(organizationId),
		enabled: Boolean(organizationId),
	});
	const hasProvisioners = provisioners ? provisioners.length > 0 : undefined;
	const showProvisionerWarning = hasProvisioners === false;

	const { setFieldValue } = form;
	// Auto-select when exactly one org is available.
	useEffect(() => {
		if (orgOptions.length === 1 && !organizationId) {
			void setFieldValue("organization_id", orgOptions[0].id);
		}
	}, [orgOptions, organizationId, setFieldValue]);

	const handleOrgChange = (org: Organization | null) => {
		void form.setFieldValue("organization_id", org?.id ?? "");
	};

	return (
		<form onSubmit={form.handleSubmit} noValidate>
			<StepFrame
				footer={
					<>
						<Button variant="outline" onClick={onBack}>
							Back
						</Button>
						<Button type="submit" disabled={hasProvisioners === false}>
							Create template
						</Button>
					</>
				}
			>
				<div className="min-w-163.5">
					{showCreateErrorAlert && (
						<ErrorAlert error={createError} className="mb-6" />
					)}
					<TemplateBuilderTitle>Customizations</TemplateBuilderTitle>
					<TemplateBuilderSubtitle>
						Add additional configurations.
					</TemplateBuilderSubtitle>

					{showProvisionerWarning && <ProvisionerWarning />}

					<div className="flex gap-8">
						{/* Base template card */}
						{state.selectedBase && (
							<BaseTemplateCard base={state.selectedBase} />
						)}

						{/* Two-column form grid */}
						<div className="grid grid-cols-2 gap-x-6 gap-y-6 content-start">
							{/* Left column */}
							<FormField
								field={displayNameField}
								label="Display name"
								id="template-display-name"
								placeholder="My template"
							/>

							{/* Right column */}
							{showOrgsError ? (
								<ErrorAlert
									error={permittedOrgsQuery.error}
									showDebugDetail={false}
									actions={
										<RetryButton
											size="sm"
											variant="outline"
											isRetrying={permittedOrgsQuery.isFetching}
											onRetry={() => void permittedOrgsQuery.refetch()}
										/>
									}
								/>
							) : (
								<FormField
									field={organizationField}
									label="Organization"
									required
									id="organization"
									control={(props) => (
										<OrganizationAutocomplete
											{...props}
											required
											autoFocus={createErrorField === "organization_id"}
											disabled={permittedOrgsQuery.isPending}
											value={selectedOrg}
											onChange={handleOrgChange}
											options={orgOptions}
										/>
									)}
								/>
							)}

							{/* Left column */}
							<div className="flex flex-col gap-2">
								<FormField
									field={descriptionField}
									label="Description"
									id="template-description"
									control={(props) => (
										<Textarea
											{...props}
											{...form.getFieldProps("description")}
											placeholder="Describe what this template is for"
											rows={3}
											className={cn(
												descriptionField.error && "border-border-destructive",
											)}
										/>
									)}
								/>
								<p className="text-xs text-content-secondary">
									{/* sentence-case-expect: Agents is the product name. */}
									Used by both humans and Agents to identify templates.
								</p>

								<IconField
									value={form.values.icon}
									error={iconField.error}
									helperText={iconField.helperText}
									onChange={(e) => {
										const target = e.target as HTMLInputElement;
										void form.setFieldValue("icon", target.value);
									}}
									onPickEmoji={(value) => {
										void form.setFieldValue("icon", value);
									}}
								/>
							</div>

							{/* Right column */}
							<FormField
								field={nameField}
								label="ID"
								required
								autoFocus={createErrorField === "name"}
								id="template-name"
								placeholder="my-template"
							/>
						</div>
					</div>
				</div>
			</StepFrame>
		</form>
	);
};

const ProvisionerWarning: React.FC = () => {
	return (
		<Alert severity="error" prominent className="my-4">
			This organization does not have any provisioners. Before you create a
			template, you&apos;ll need to configure a provisioner.{" "}
			<Link
				href={docs("/admin/provisioners#organization-scoped-provisioners")}
				target="_blank"
				rel="noreferrer"
			>
				See our documentation
			</Link>
		</Alert>
	);
};

const BaseTemplateCard: React.FC<{ base: SelectedBaseMeta }> = ({ base }) => {
	return (
		<div className="w-56 shrink-0 rounded-lg bg-surface-secondary p-4 self-start">
			{base.iconUrl && <Avatar src={base.iconUrl} size="lg" variant="icon" />}
			<p className="text-sm font-bold text-content-primary">{base.name}</p>
			<p className="text-xs text-content-secondary mt-1">
				Preset based on base template
			</p>
		</div>
	);
};
