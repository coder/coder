import { type FC, useEffect } from "react";
import { useQuery } from "react-query";
import {
	permittedOrganizations,
	provisionerDaemons,
} from "#/api/queries/organizations";
import type { Organization } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import { IconField } from "#/components/IconField/IconField";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Link } from "#/components/Link/Link";
import { OrganizationAutocomplete } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import {
	TemplateBuilderSubtitle,
	TemplateBuilderTitle,
} from "#/pages/TemplateBuilder/TemplateBuilderHeader";
import { cn } from "#/utils/cn";
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

/**
 * Returns true when required customization fields are set and the selected
 * organization has provisioners (or provisioner status is still unknown).
 */
export function customizationsComplete(
	state: Pick<
		TemplateBuilderWizardState,
		"name" | "organizationId" | "hasProvisioners"
	>,
): boolean {
	return (
		state.name.trim() !== "" &&
		Boolean(state.organizationId) &&
		state.hasProvisioners !== false
	);
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
	const selectedOrg =
		orgOptions.find((org) => org.id === state.organizationId) ?? null;
	const autoSelectedSingleOrg =
		permittedOrgsQuery.isSuccess && orgOptions.length === 1;

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

	// Keep the wizard's organization in sync with the fetched permitted orgs:
	// auto-select the sole option, and drop a selection that a later refetch no
	// longer permits so a stale organization can't be submitted.
	useEffect(() => {
		const orgs = permittedOrgsQuery.data;
		if (!orgs) {
			return;
		}
		if (orgs.length === 1) {
			if (state.organizationId !== orgs[0].id) {
				onChangeField("organizationId", orgs[0].id);
			}
			return;
		}
		if (
			state.organizationId &&
			!orgs.some((org) => org.id === state.organizationId)
		) {
			onChangeField("organizationId", "");
		}
	}, [permittedOrgsQuery.data, state.organizationId, onChangeField]);

	const handleOrgChange = (org: Organization | null) => {
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
					<div
						className={cn(
							"flex flex-col gap-2",
							autoSelectedSingleOrg && "col-span-2",
						)}
					>
						<Label htmlFor="template-display-name">Display name</Label>
						<Input
							id="template-display-name"
							value={state.displayName}
							onChange={(e) => onChangeField("displayName", e.target.value)}
							placeholder="My Template"
						/>
					</div>

					{!autoSelectedSingleOrg && (
						<div className="flex flex-col gap-2">
							<Label htmlFor="organization">
								Organization
								<span className="text-xs font-bold text-content-destructive ml-1">
									*
								</span>
							</Label>
							{permittedOrgsQuery.isLoading ? (
								<div className="flex h-10 items-center">
									<Spinner
										loading
										size="sm"
										aria-label="Loading organizations"
									/>
								</div>
							) : permittedOrgsQuery.isError ? (
								<div className="flex flex-col items-start gap-2">
									<p className="text-xs text-content-destructive">
										Failed to load organizations.
									</p>
									<Button
										variant="outline"
										size="sm"
										onClick={() => permittedOrgsQuery.refetch()}
									>
										Retry
									</Button>
								</div>
							) : orgOptions.length === 0 ? (
								<p className="text-xs text-content-secondary">
									You do not have permission to create templates in any
									organization.
								</p>
							) : (
								<OrganizationAutocomplete
									id="organization"
									required
									value={selectedOrg}
									onChange={handleOrgChange}
									options={orgOptions}
								/>
							)}
						</div>
					)}

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
