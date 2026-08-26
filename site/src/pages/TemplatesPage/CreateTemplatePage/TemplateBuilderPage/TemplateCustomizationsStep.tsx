import { useFormik } from "formik";
import { type FC, useEffect, useState } from "react";
import { useQuery } from "react-query";
import * as Yup from "yup";
import {
	permittedOrganizations,
	provisionerDaemons,
} from "#/api/queries/organizations";
import type { Organization } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { Avatar } from "#/components/Avatar/Avatar";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import { Label } from "#/components/Label/Label";
import { Link } from "#/components/Link/Link";
import { OrganizationAutocomplete } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { Textarea } from "#/components/Textarea/Textarea";
import {
	TemplateBuilderSubtitle,
	TemplateBuilderTitle,
} from "#/pages/TemplatesPage/CreateTemplatePage/TemplateBuilderPage/TemplateBuilderHeader";
import { cn } from "#/utils/cn";
import { docs } from "#/utils/docs";
import {
	displayNameValidator,
	getFormHelpers,
	iconValidator,
	nameValidator,
} from "#/utils/formUtils";
import type {
	CustomizationsFormValues,
	SelectedBaseMeta,
	TemplateBuilderWizardState,
} from "./wizardState";

export const TEMPLATE_CUSTOMIZATIONS_FORM_ID = "template-customizations-form";

const MAX_DESCRIPTION_CHAR_LIMIT = 128;

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

interface TemplateCustomizationsStepProps {
	state: TemplateBuilderWizardState;
	onCreate: (values: CustomizationsFormValues) => void;
	onProvisionerStatusChange: (hasProvisioners: boolean | undefined) => void;
}

export const TemplateCustomizationsStep: FC<
	TemplateCustomizationsStepProps
> = ({ state, onCreate, onProvisionerStatusChange }) => {
	const permittedOrgsQuery = useQuery(
		permittedOrganizations({
			object: { resource_type: "template" },
			action: "create",
		}),
	);
	const orgOptions = permittedOrgsQuery.data ?? [];

	const form = useFormik<CustomizationsFormValues>({
		initialValues: {
			organization_id: "",
			name: state.name,
			display_name: state.displayName,
			description: state.description,
			icon: state.icon,
		},
		validationSchema,
		onSubmit: (values) => onCreate(values),
	});
	const getFieldHelpers = getFormHelpers(form);
	const descriptionField = getFieldHelpers("description");
	const iconField = getFieldHelpers("icon");
	const organizationField = getFieldHelpers("organization_id");

	// Display object for the autocomplete; the Formik field only stores the id.
	const [selectedOrg, setSelectedOrg] = useState<Organization | null>(null);

	const { data: provisioners } = useQuery({
		...provisionerDaemons(selectedOrg?.id ?? ""),
		enabled: Boolean(selectedOrg),
	});
	const hasProvisioners = provisioners ? provisioners.length > 0 : undefined;
	const showProvisionerWarning = hasProvisioners === false;

	// Notify parent when provisioner status changes so the wizard can
	// disable the create button when no provisioners are available.
	useEffect(() => {
		onProvisionerStatusChange(hasProvisioners);
	}, [hasProvisioners, onProvisionerStatusChange]);

	// Auto-select when exactly one org is available.
	useEffect(() => {
		if (orgOptions.length === 1 && !selectedOrg) {
			setSelectedOrg(orgOptions[0]);
			void form.setFieldValue("organization_id", orgOptions[0].id);
		}
	}, [orgOptions, selectedOrg, form]);

	const handleOrgChange = (org: Organization | null) => {
		setSelectedOrg(org);
		void form.setFieldValue("organization_id", org?.id ?? "");
	};

	return (
		<form
			id={TEMPLATE_CUSTOMIZATIONS_FORM_ID}
			onSubmit={form.handleSubmit}
			noValidate
			className="min-w-[654px]"
		>
			<TemplateBuilderTitle>Customizations</TemplateBuilderTitle>
			<TemplateBuilderSubtitle>
				Add additional configurations.
			</TemplateBuilderSubtitle>

			{showProvisionerWarning && <ProvisionerWarning />}

			<div className="flex gap-8">
				{/* Base template card */}
				{state.selectedBase && <BaseTemplateCard base={state.selectedBase} />}

				{/* Two-column form grid */}
				<div className="grid grid-cols-2 gap-x-6 gap-y-6 content-start">
					{/* Left column */}
					<FormField
						field={getFieldHelpers("display_name")}
						label="Display name"
						id="template-display-name"
						placeholder="My Template"
					/>

					{/* Right column */}
					{orgOptions.length > 0 && (
						<div className="flex flex-col gap-2">
							<Label htmlFor="organization">
								Organization
								<span className="text-xs font-bold text-content-destructive ml-1">
									*
								</span>
							</Label>
							<OrganizationAutocomplete
								id="organization"
								required
								value={selectedOrg}
								onChange={handleOrgChange}
								options={orgOptions}
							/>
							{organizationField.error && (
								<span className="text-xs text-content-destructive">
									{organizationField.helperText}
								</span>
							)}
						</div>
					)}

					{/* Left column */}
					<div className="flex flex-col gap-2">
						<Label htmlFor="template-description">Description</Label>
						<Textarea
							{...form.getFieldProps("description")}
							id="template-description"
							placeholder="Describe what this template is for"
							rows={3}
							aria-invalid={descriptionField.error}
							className={cn(
								descriptionField.error && "border-border-destructive",
							)}
						/>
						{descriptionField.error && (
							<span className="text-xs text-content-destructive">
								{descriptionField.helperText}
							</span>
						)}
						<p className="text-xs text-content-secondary">
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
						field={getFieldHelpers("name", {
							helperText: "Used to identify the template in URLs and the API.",
						})}
						label="ID"
						required
						id="template-name"
						placeholder="my-template"
					/>
				</div>
			</div>
		</form>
	);
};

const ProvisionerWarning: FC = () => {
	return (
		<Alert severity="error" prominent className="my-4">
			This organization does not have any provisioners. Before you create a
			template, you&apos;ll need to configure a provisioner.{" "}
			<Link href={docs("/admin/provisioners#organization-scoped-provisioners")}>
				See our documentation
			</Link>
		</Alert>
	);
};

const BaseTemplateCard: FC<{ base: SelectedBaseMeta }> = ({ base }) => {
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
