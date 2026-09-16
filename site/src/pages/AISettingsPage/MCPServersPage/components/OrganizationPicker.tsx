import type { FC } from "react";
import type { Organization } from "#/api/typesGenerated";
import { OrganizationField } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";

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
}) => (
	<OrganizationField
		id={id}
		organizations={organizations}
		organization={organization}
		onChange={onChange}
		className={className}
		disabled={disabled}
		showLabel={showLabel}
		showSingleOrganization={showSingleOrganization}
	/>
);
