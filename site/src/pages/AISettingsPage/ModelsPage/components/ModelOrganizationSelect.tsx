import { type FC, useId } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import { Label } from "#/components/Label/Label";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
	OrganizationValue,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import {
	modelOrganizationSearchParam,
	useOrganizationModels,
} from "../organizationModels";

// Switches the active organization via the org search param.
export const ModelOrganizationSelect: FC<{
	label?: string;
	readOnly?: boolean;
	requireCreatePermission?: boolean;
	triggerClassName?: string;
}> = ({
	label,
	readOnly = false,
	requireCreatePermission = false,
	triggerClassName,
}) => {
	const { organization, accessibleOrganizations, permissionsByOrganization } =
		useOrganizationModels();
	const selectableOrganizations = requireCreatePermission
		? accessibleOrganizations.filter(
				(organization) =>
					permissionsByOrganization?.[organization.id]?.createChatModelConfigs,
			)
		: accessibleOrganizations;
	const location = useLocation();
	const navigate = useNavigate();
	const [searchParams] = useSearchParams();
	const id = useId();

	if (!readOnly && selectableOrganizations.length <= 1) {
		return null;
	}

	const autocomplete = readOnly ? (
		<OrganizationValue
			id={id}
			organization={organization}
			labelOrganizations={accessibleOrganizations}
		/>
	) : (
		<OrganizationAutocomplete
			id={id}
			value={organization}
			ariaLabel={`Organization ${getOrganizationLabel(
				organization,
				accessibleOrganizations,
			)}`}
			options={selectableOrganizations}
			triggerClassName={triggerClassName}
			optionsTabbable
			onChange={(nextOrganization) => {
				if (!nextOrganization) {
					return;
				}
				const next = new URLSearchParams(searchParams);
				next.set(modelOrganizationSearchParam, nextOrganization.name);
				void navigate(`${location.pathname}?${next.toString()}`);
			}}
		/>
	);

	if (!label) {
		return autocomplete;
	}

	return (
		<div className="grid gap-1.5">
			<Label htmlFor={id} className="leading-6 text-content-primary">
				{label}
			</Label>
			{autocomplete}
		</div>
	);
};
