import { type FC, useId } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import { Label } from "#/components/Label/Label";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import {
	modelOrganizationSearchParam,
	useOrganizationModels,
} from "../organizationModels";

// Switches the active organization by rewriting the org search param while
// preserving the current path and auxiliary parameters. Hidden when only one
// organization is accessible.
export const ModelOrganizationSelect: FC<{
	label?: string;
	triggerClassName?: string;
}> = ({ label, triggerClassName }) => {
	const { organization, accessibleOrganizations } = useOrganizationModels();
	const location = useLocation();
	const navigate = useNavigate();
	const [searchParams] = useSearchParams();
	const id = useId();

	if (accessibleOrganizations.length <= 1) {
		return null;
	}

	const autocomplete = (
		<OrganizationAutocomplete
			id={id}
			value={organization}
			ariaLabel={`Organization ${getOrganizationLabel(
				organization,
				accessibleOrganizations,
			)}`}
			options={accessibleOrganizations}
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
