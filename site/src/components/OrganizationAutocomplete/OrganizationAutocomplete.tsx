import { css } from "@emotion/css";
import Autocomplete from "@mui/material/Autocomplete";
import CircularProgress from "@mui/material/CircularProgress";
import TextField from "@mui/material/TextField";
import { checkAuthorization } from "api/queries/authCheck";
import { organizations } from "api/queries/organizations";
import type { AuthorizationCheck, Organization } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { type ComponentProps, type FC, useEffect, useState } from "react";
import { useQuery } from "react-query";

export type OrganizationAutocompleteProps = {
	onChange: (organization: Organization | null) => void;
	label?: string;
	className?: string;
	size?: ComponentProps<typeof TextField>["size"];
	required?: boolean;
	check?: AuthorizationCheck;
};

export const OrganizationAutocomplete: FC<OrganizationAutocompleteProps> = ({
	onChange,
	label,
	className,
	size = "small",
	required,
	check,
}) => {
	const [open, setOpen] = useState(false);
	const [selected, setSelected] = useState<Organization | null>(null);

	const organizationsQuery = useQuery(organizations());

	const permissionsQuery = useQuery(
		check && organizationsQuery.data
			? checkAuthorization({
					checks: Object.fromEntries(
						organizationsQuery.data.map((org) => [
							org.id,
							{
								...check,
								object: { ...check.object, organization_id: org.id },
							},
						]),
					),
				})
			: { enabled: false },
	);

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
		<Autocomplete
			noOptionsText="No organizations found"
			className={className}
			options={options}
			disabled={options.length === 1}
			value={selected}
			loading={organizationsQuery.isLoading}
			data-testid="organization-autocomplete"
			open={open}
			isOptionEqualToValue={(a, b) => a.id === b.id}
			getOptionLabel={(option) => option.display_name}
			onOpen={() => {
				setOpen(true);
			}}
			onClose={() => {
				setOpen(false);
			}}
			onChange={(_, newValue) => {
				setSelected(newValue);
				onChange(newValue);
			}}
			renderOption={({ key, ...props }, option) => (
				<li key={key} {...props}>
					<AvatarData
						title={option.display_name}
						subtitle={option.name}
						src={option.icon}
					/>
				</li>
			)}
			renderInput={(params) => (
				<TextField
					{...params}
					required={required}
					fullWidth
					size={size}
					label={label}
					placeholder="Organization name"
					css={{
						"&:not(:has(label))": {
							margin: 0,
						},
					}}
					InputProps={{
						...params.InputProps,
						startAdornment: selected && (
							<Avatar size="sm" src={selected.icon} fallback={selected.name} />
						),
						endAdornment: (
							<>
								{organizationsQuery.isFetching && open && (
									<CircularProgress size={16} />
								)}
								{params.InputProps.endAdornment}
							</>
						),
						classes: { root },
					}}
					InputLabelProps={{
						shrink: true,
					}}
				/>
			)}
		/>
	);
};

const root = css`
	padding-left: 14px !important; // Same padding left as input
	gap: 4px;
`;
