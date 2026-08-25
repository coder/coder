import type { FC } from "react";
import type { Organization } from "#/api/typesGenerated";
import { Label } from "#/components/Label/Label";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
	OrganizationValue,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { cn } from "#/utils/cn";

interface OrganizationPickerProps {
	id: string;
	organizations: readonly Organization[];
	organization: Organization;
	onChange?: (organization: Organization) => void;
	className?: string;
	disabled?: boolean;
	showLabel?: boolean;
	showSingleOrganization?: boolean;
}

export const OrganizationPicker: FC<OrganizationPickerProps> = ({
	id,
	organizations,
	organization,
	onChange,
	className,
	disabled,
	showLabel = true,
	showSingleOrganization = false,
}) => {
	const hasSingleSelectedOrganization =
		organizations.length <= 1 &&
		organizations.some((option) => option.id === organization.id);
	if (hasSingleSelectedOrganization && !showSingleOrganization) {
		return null;
	}

	// The selected organization can fall outside the selectable options,
	// such as a deep link to an organization where servers are listable
	// but not creatable, so include it when disambiguating labels.
	const labelOrganizations = organizations.some(
		(option) => option.id === organization.id,
	)
		? organizations
		: [...organizations, organization];
	const organizationLabel = getOrganizationLabel(
		organization,
		labelOrganizations,
	);
	// A picker without a change handler or alternatives is informational, so
	// render a static value instead of a disabled control with muted text.
	const isReadOnly = !onChange || hasSingleSelectedOrganization;

	return (
		<div className={cn("flex w-72 flex-col gap-2", className)}>
			{showLabel && <Label htmlFor={id}>Organization</Label>}
			{isReadOnly ? (
				<OrganizationValue
					id={id}
					organization={organization}
					labelOrganizations={labelOrganizations}
				/>
			) : (
				<OrganizationAutocomplete
					id={id}
					ariaLabel={`Organization ${organizationLabel}`}
					value={organization}
					onChange={(org) => {
						if (org) {
							onChange?.(org);
						}
					}}
					options={organizations}
					labelOrganizations={labelOrganizations}
					required
					disabled={disabled}
				/>
			)}
		</div>
	);
};
