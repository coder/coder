import { checkAuthorization } from "api/queries/authCheck";
import { organizations } from "api/queries/organizations";
import type { AuthorizationCheck, Organization } from "api/typesGenerated";
import { Autocomplete } from "components/Autocomplete/Autocomplete";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { type FC, useEffect, useState } from "react";
import { useQuery } from "react-query";

type OrganizationAutocompleteProps = {
	onChange: (organization: Organization | null) => void;
	label?: string;
	className?: string;
	required?: boolean;
	check?: AuthorizationCheck;
	error?: boolean;
	helperText?: React.ReactNode;
};

export const OrganizationAutocomplete: FC<OrganizationAutocompleteProps> = ({
	onChange,
	label,
	className,
	required,
	check,
	error,
	helperText,
}) => {
	const [selected, setSelected] = useState<Organization | null>(null);
	const organizationsQuery = useQuery(organizations());
	const checks =
		check &&
		organizationsQuery.data &&
		Object.fromEntries(
			organizationsQuery.data.map((org) => [
				org.id,
				{
					...check,
					object: { ...check.object, organization_id: org.id },
				},
			]),
		);

	const permissionsQuery = useQuery({
		...checkAuthorization({
			checks: checks ?? {},
		}),
		enabled: Boolean(check && organizationsQuery.data),
	});

	// If an authorization check was provided, filter the organizations based on
	// the results of that check.
	let options = organizationsQuery.data ?? [];
	if (check) {
		options = permissionsQuery.data
			? options.filter((org) => permissionsQuery.data[org.id])
			: [];
	}

	// Unfortunate: this useEffect sets a default org value
	// if only one is available and is necessary as the autocomplete loads
	// its own data. Until we refactor, proceed cautiously!
	useEffect(() => {
		const org = options[0];
		if (options.length !== 1 || org === selected) {
			return;
		}

		setSelected(org);
		onChange(org);
	}, [options, selected, onChange]);

	return (
		<div className="flex flex-col gap-2">
			<Autocomplete
				value={selected}
				onChange={(newValue) => {
					setSelected(newValue);
					onChange(newValue);
				}}
				options={options}
				getOptionValue={(option) => option.id}
				getOptionLabel={(option) => option.display_name}
				isOptionEqualToValue={(a, b) => a.id === b.id}
				renderOption={(option) => (
					<AvatarData
						title={option.display_name}
						subtitle={option.name}
						src={option.icon}
					/>
				)}
				label={label}
				placeholder="Organization name"
				noOptionsText="No organizations found"
				loading={organizationsQuery.isLoading}
				disabled={options.length === 1}
				required={required}
				startAdornment={
					selected && (
						<Avatar size="sm" src={selected.icon} fallback={selected.name} />
					)
				}
				className={className}
				data-testid="organization-autocomplete"
			/>
			{helperText && (
				<span
					className={`text-xs ${error ? "text-content-destructive" : "text-content-secondary"}`}
				>
					{helperText}
				</span>
			)}
		</div>
	);
};
