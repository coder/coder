import type { FC } from "react";
import type { Organization } from "#/api/typesGenerated";
import { Label } from "#/components/Label/Label";
import { OrganizationAutocomplete } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { cn } from "#/utils/cn";

interface OrganizationPickerProps {
	id: string;
	organizations: readonly Organization[];
	organization: Organization | undefined;
	onChange: (organization: Organization) => void;
	className?: string;
}

export const OrganizationPicker: FC<OrganizationPickerProps> = ({
	id,
	organizations,
	organization,
	onChange,
	className,
}) => {
	if (organizations.length <= 1) {
		return null;
	}

	return (
		<div className={cn("flex w-72 flex-col gap-2", className)}>
			<Label htmlFor={id}>Organization</Label>
			<OrganizationAutocomplete
				id={id}
				value={organization ?? null}
				onChange={(org) => {
					if (org) {
						onChange(org);
					}
				}}
				options={organizations}
				required
			/>
		</div>
	);
};
