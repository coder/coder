import { css } from "@emotion/css";
import Autocomplete from "@mui/material/Autocomplete";
import CircularProgress from "@mui/material/CircularProgress";
import TextField from "@mui/material/TextField";
import { checkAuthorization } from "api/queries/authCheck";
import { organizations } from "api/queries/organizations";
import type { AuthorizationCheck, Organization } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { useDebouncedFunction } from "hooks/debounce";
import {
	type ChangeEvent,
	type ComponentProps,
	type FC,
	useState,
} from "react";
import { useQuery } from "react-query";

export type OrganizationAutocompleteProps = {
	value: Organization | null;
	onChange: (organization: Organization | null) => void;
	label?: string;
	className?: string;
	size?: ComponentProps<typeof TextField>["size"];
	required?: boolean;
	check?: AuthorizationCheck;
};

export const OrganizationAutocomplete: FC<OrganizationAutocompleteProps> = ({
	value,
	onChange,
	label,
	className,
	size = "small",
	required,
	check,
}) => {
	const [autoComplete, setAutoComplete] = useState<{
		value: string;
		open: boolean;
	}>({
		value: value?.name ?? "",
		open: false,
	});
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

	const { debounced: debouncedInputOnChange } = useDebouncedFunction(
		(event: ChangeEvent<HTMLInputElement>) => {
			setAutoComplete((state) => ({
				...state,
				value: event.target.value,
			}));
		},
		750,
	);

	// If an authorization check was provided, filter the organizations based on
	// the results of that check.
	let options = organizationsQuery.data ?? [];
	if (check) {
		options = permissionsQuery.data
			? options.filter((org) => permissionsQuery.data[org.id])
			: [];
	}

	return (
		<Autocomplete
			noOptionsText="No organizations found"
			className={className}
			options={options}
			loading={organizationsQuery.isLoading}
			value={value}
			data-testid="organization-autocomplete"
			open={autoComplete.open}
			isOptionEqualToValue={(a, b) => a.name === b.name}
			getOptionLabel={(option) => option.display_name}
			onOpen={() => {
				setAutoComplete((state) => ({
					...state,
					open: true,
				}));
			}}
			onClose={() => {
				setAutoComplete({
					value: value?.name ?? "",
					open: false,
				});
			}}
			onChange={(_, newValue) => {
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
					autoFocus
					placeholder="Organization name"
					css={{
						"&:not(:has(label))": {
							margin: 0,
						},
					}}
					InputProps={{
						...params.InputProps,
						onChange: debouncedInputOnChange,
						startAdornment: value && (
							<Avatar size="sm" src={value.icon} fallback={value.name} />
						),
						endAdornment: (
							<>
								{organizationsQuery.isFetching && autoComplete.open && (
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
