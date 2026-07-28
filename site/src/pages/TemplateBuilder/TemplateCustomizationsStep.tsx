import { type FC, useEffect, useState } from "react";
import { useQuery } from "react-query";
import {
	permittedOrganizations,
	provisionerDaemons,
} from "#/api/queries/organizations";
import type { Organization } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { Avatar } from "#/components/Avatar/Avatar";
import { IconField } from "#/components/IconField/IconField";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Link } from "#/components/Link/Link";
import { OrganizationAutocomplete } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { Textarea } from "#/components/Textarea/Textarea";
import {
	TemplateBuilderSubtitle,
	TemplateBuilderTitle,
} from "#/pages/TemplateBuilder/TemplateBuilderHeader";
import { docs } from "#/utils/docs";
import type {
	SelectedBaseMeta,
	TemplateBuilderWizardState,
} from "./wizardState";

interface TemplateCustomizationsStepProps {
	state: TemplateBuilderWizardState;
	onChangeField: (
		field: "organizationId" | "name" | "displayName" | "description" | "icon",
		value: string,
	) => void;
	onProvisionerStatusChange: (hasProvisioners: boolean | undefined) => void;
}

export const TemplateCustomizationsStep: FC<
	TemplateCustomizationsStepProps
> = ({ state, onChangeField, onProvisionerStatusChange }) => {
	const permittedOrgsQuery = useQuery(
		permittedOrganizations({
			object: { resource_type: "template" },
			action: "create",
		}),
	);
	const orgOptions = permittedOrgsQuery.data ?? [];

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
			onChangeField("organizationId", orgOptions[0].id);
		}
	}, [orgOptions, selectedOrg, onChangeField]);

	const handleOrgChange = (org: Organization | null) => {
		setSelectedOrg(org);
		onChangeField("organizationId", org?.id ?? "");
	};

	return (
		<div className="min-w-[654px]">
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
					<div className="flex flex-col gap-2">
						<Label htmlFor="template-display-name">Display name</Label>
						<Input
							id="template-display-name"
							value={state.displayName}
							onChange={(e) => onChangeField("displayName", e.target.value)}
							placeholder="My Template"
						/>
					</div>

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
						</div>
					)}

					{/* Left column */}
					<div className="flex flex-col gap-2">
						<Label htmlFor="template-description">Description</Label>
						<Textarea
							id="template-description"
							value={state.description}
							onChange={(e) => onChangeField("description", e.target.value)}
							placeholder="Describe what this template is for"
							rows={3}
						/>
						<p className="text-xs text-content-secondary">
							Used by both humans and Agents to identify templates.
						</p>

						<IconField
							value={state.icon}
							onChange={(e) => {
								const target = e.target as HTMLInputElement;
								onChangeField("icon", target.value);
							}}
							onPickEmoji={(value) => onChangeField("icon", value)}
						/>
					</div>

					{/* Right column */}
					<div className="flex flex-col gap-2">
						<Label htmlFor="template-name">
							ID
							<span className="text-xs font-bold text-content-destructive ml-1">
								*
							</span>
						</Label>
						<Input
							id="template-name"
							value={state.name}
							onChange={(e) => onChangeField("name", e.target.value)}
							placeholder="my-template"
							aria-required
						/>
						<p className="text-xs text-content-secondary">
							Used to identify the template in URLs and the API.
						</p>
					</div>
				</div>
			</div>
		</div>
	);
};

const ProvisionerWarning: FC = () => {
	return (
		<Alert severity="warning" prominent className="my-4">
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
			<p className="text-sm font-medium text-content-primary">{base.name}</p>
			<p className="text-xs text-content-secondary mt-1">
				Preset based on base template
			</p>
		</div>
	);
};
