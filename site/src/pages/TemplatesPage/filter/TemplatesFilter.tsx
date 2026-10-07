import { Building2Icon, SlidersHorizontalIcon, UserIcon } from "lucide-react";
import { useMemo } from "react";
import { useQueryClient } from "react-query";
import {
	getValidationErrorMessage,
	hasError,
	isApiValidationError,
} from "#/api/errors";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import type { FilterCategory } from "#/components/Filter/FilterCombobox/types";
import {
	getSelfUserFilterOptions,
	getUserFilterOptions,
} from "#/components/Filter/userFilterOptions";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import {
	ATTRIBUTE_CHIP_KEYS,
	getAttributeFilterOptions,
	getOrganizationFilterOptions,
} from "./categoryOptions";

type TemplatesFilterProps = Readonly<{
	filter: UseFilterResult;
	error: unknown;
}>;

export const TemplatesFilter: React.FC<TemplatesFilterProps> = ({
	filter,
	error,
}) => {
	const { showOrganizations, organizations } = useDashboard();
	const { permissions, user: me } = useAuthenticated();
	const canListUsers = permissions.viewAllUsers;
	const queryClient = useQueryClient();

	const categories = useMemo(() => {
		const next: FilterCategory[] = [
			{
				key: "attributes",
				label: "Attributes",
				icon: <SlidersHorizontalIcon />,
				chipKeys: ATTRIBUTE_CHIP_KEYS,
				getOptions: getAttributeFilterOptions,
			},
		];

		if (showOrganizations) {
			next.push({
				key: "organization",
				label: "Organization",
				icon: <Building2Icon />,
				getOptions: (query) =>
					getOrganizationFilterOptions(query, organizations),
			});
		}

		// Always expose Author so `author` stays a recognized chip key and
		// `author:me` renders as a chip rather than free text. Users who cannot
		// list others only see themselves.
		next.push({
			key: "author",
			label: "Author",
			icon: <UserIcon />,
			getOptions: canListUsers
				? (query) => getUserFilterOptions(query, me, queryClient)
				: (query) => getSelfUserFilterOptions(query, me),
		});

		return next;
	}, [canListUsers, me, organizations, showOrganizations, queryClient]);

	const showValidationError = hasError(error) && isApiValidationError(error);

	return (
		<div className="flex min-w-0 flex-col gap-2">
			<FilterCombobox
				queryScope="templates"
				value={filter.query}
				onChange={filter.update}
				categories={categories}
				placeholder="Search and filter templates…"
				className="w-full min-w-0 self-start sm:w-auto sm:min-w-lg sm:max-w-full"
				errorMessage={
					showValidationError ? getValidationErrorMessage(error) : undefined
				}
			/>
		</div>
	);
};
