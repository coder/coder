import { type FC, useEffect, useState } from "react";
import { useQuery } from "react-query";
import { permittedOrganizations } from "#/api/queries/organizations";
import type { Organization } from "#/api/typesGenerated";
import { IconField } from "#/components/IconField/IconField";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { OrganizationAutocomplete } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import type { TemplateBuilderWizardState } from "./wizardState";

interface TemplateCustomizationsStepProps {
	state: TemplateBuilderWizardState;
	onChangeField: (
		field: "organizationId" | "name" | "displayName" | "description" | "icon",
		value: string,
	) => void;
}

export const TemplateCustomizationsStep: FC<
	TemplateCustomizationsStepProps
> = ({ state, onChangeField }) => {
	const permittedOrgsQuery = useQuery(
		permittedOrganizations({
			object: { resource_type: "template" },
			action: "create",
		}),
	);
	const orgOptions = permittedOrgsQuery.data ?? [];

	const [selectedOrg, setSelectedOrg] = useState<Organization | null>(null);

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
		<div>
			<h2 className="text-lg font-semibold mb-1">Template details</h2>
			<p className="text-sm text-content-secondary mb-4">
				Configure your new template.
			</p>

			<div className="flex flex-col gap-6">
				{orgOptions.length > 1 && (
					<div className="flex flex-col gap-2">
						<Label htmlFor="organization">Organization</Label>
						<OrganizationAutocomplete
							id="organization"
							required
							value={selectedOrg}
							onChange={handleOrgChange}
							options={orgOptions}
						/>
					</div>
				)}

				<div className="flex flex-col gap-2">
					<Label htmlFor="template-name">
						Name
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
				</div>

				<div className="flex flex-col gap-2">
					<Label htmlFor="template-display-name">Display name</Label>
					<Input
						id="template-display-name"
						value={state.displayName}
						onChange={(e) => onChangeField("displayName", e.target.value)}
						placeholder="My Template"
					/>
				</div>

				<div className="flex flex-col gap-2">
					<Label htmlFor="template-description">Description</Label>
					<textarea
						id="template-description"
						value={state.description}
						onChange={(e) => onChangeField("description", e.target.value)}
						placeholder="Describe what this template is for"
						rows={3}
						className="flex w-full rounded-md border border-border border-solid bg-transparent px-3 py-2 text-sm shadow-sm placeholder:text-content-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link disabled:cursor-not-allowed disabled:opacity-50"
					/>
				</div>

				<div className="flex flex-col gap-2">
					<IconField
						value={state.icon}
						onChange={(e) => {
							const target = e.target as HTMLInputElement;
							onChangeField("icon", target.value);
						}}
						onPickEmoji={(value) => onChangeField("icon", value)}
					/>
				</div>
			</div>
		</div>
	);
};
