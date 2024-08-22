import { css } from "@emotion/css";
import Autocomplete from "@mui/material/Autocomplete";
import CircularProgress from "@mui/material/CircularProgress";
import TextField from "@mui/material/TextField";
import { organizationMembers } from "api/queries/organizations";
import type { OrganizationMemberWithUserData } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import { useDebouncedFunction } from "hooks/debounce";
import {
	type ChangeEvent,
	type ComponentProps,
	type FC,
	useState,
} from "react";
import { useQuery } from "react-query";

export type MemberAutocompleteProps = {
	className?: string;
	onChange: (user: OrganizationMemberWithUserData | null) => void;
	organizationId: string;
	value: OrganizationMemberWithUserData | null;
};

export const MemberAutocomplete: FC<MemberAutocompleteProps> = ({
	className,
	onChange,
	organizationId,
	value,
}) => {
	const [autoComplete, setAutoComplete] = useState<{
		value: string;
		open: boolean;
	}>({
		value: value?.email ?? "",
		open: false,
	});

	// Currently this queries all members, as there is no pagination.
	const membersQuery = useQuery({
		...organizationMembers(organizationId),
		enabled: autoComplete.open,
		keepPreviousData: true,
	});

	const { debounced: debouncedInputOnChange } = useDebouncedFunction(
		(event: ChangeEvent<HTMLInputElement>) => {
			setAutoComplete((state) => ({
				...state,
				value: event.target.value,
			}));
		},
		750,
	);

	return (
		<Autocomplete
			noOptionsText="No organization members found"
			className={className}
			options={membersQuery.data ?? []}
			loading={membersQuery.isLoading}
			value={value}
			data-testid="user-autocomplete"
			open={autoComplete.open}
			isOptionEqualToValue={(a, b) => a.username === b.username}
			getOptionLabel={(option) => option.email}
			onOpen={() => {
				setAutoComplete((state) => ({
					...state,
					open: true,
				}));
			}}
			onClose={() => {
				setAutoComplete({
					value: value?.email ?? "",
					open: false,
				});
			}}
			onChange={(_, newValue) => {
				onChange(newValue);
			}}
			renderOption={({ key, ...props }, option) => (
				<li key={key} {...props}>
					<AvatarData
						title={option.username}
						subtitle={option.email}
						src={option.avatar_url}
					/>
				</li>
			)}
			renderInput={(params) => (
				<TextField
					{...params}
					fullWidth
					placeholder="Organization member email or username"
					css={{
						"&:not(:has(label))": {
							margin: 0,
						},
					}}
					InputProps={{
						...params.InputProps,
						onChange: debouncedInputOnChange,
						startAdornment: value && (
							<Avatar size="sm" src={value.avatar_url}>
								{value.username}
							</Avatar>
						),
						endAdornment: (
							<>
								{membersQuery.isFetching && autoComplete.open && (
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
