import { CircleCheck, X } from "lucide-react";
import type { FC } from "react";
import {
	Filter,
	MenuSkeleton,
	type useFilter,
} from "#/components/Filter/Filter";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "#/components/Filter/menu";
import {
	SelectFilter,
	type SelectFilterOption,
} from "#/components/Filter/SelectFilter";
import { StatusIndicatorDot } from "#/components/StatusIndicator/StatusIndicator";
import { docs } from "#/utils/docs";

const userFilterQuery = {
	active: "status:active",
	aiAddon: "has-ai-seat:true",
	serviceAccount: "service_account:true",
	all: "",
};

export const useStatusFilterMenu = ({
	value,
	onChange,
}: Pick<UseFilterMenuOptions, "value" | "onChange">) => {
	const statusOptions: SelectFilterOption[] = [
		{
			value: "active",
			label: "Active",
			startIcon: <StatusIndicatorDot variant="success" />,
		},
		{
			value: "dormant",
			label: "Dormant",
			startIcon: <StatusIndicatorDot variant="warning" />,
		},
		{
			value: "suspended",
			label: "Suspended",
			startIcon: <StatusIndicatorDot variant="inactive" />,
		},
	];
	return useFilterMenu({
		onChange,
		value,
		id: "status",
		getSelectedOption: async () =>
			statusOptions.find((option) => option.value === value) ?? null,
		getOptions: async () => statusOptions,
	});
};

export const useAISeatFilterMenu = ({
	value,
	onChange,
}: Pick<UseFilterMenuOptions, "value" | "onChange">) => {
	const aiSeatOptions: SelectFilterOption[] = [
		{
			value: "true",
			label: "Consuming AI seat",
			startIcon: <CircleCheck className="size-icon-sm text-content-success" />,
		},
		{
			value: "false",
			label: "Not consuming AI seat",
			startIcon: <X className="size-icon-sm text-content-disabled" />,
		},
	];
	return useFilterMenu({
		onChange,
		value,
		id: "has-ai-seat",
		getSelectedOption: async () =>
			aiSeatOptions.find((option) => option.value === value) ?? null,
		getOptions: async () => aiSeatOptions,
	});
};

type StatusFilterMenu = ReturnType<typeof useStatusFilterMenu>;
type AISeatFilterMenu = ReturnType<typeof useAISeatFilterMenu>;

interface UsersFilterProps {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus?: {
		status?: StatusFilterMenu;
		aiSeat?: AISeatFilterMenu;
	};
}

export const UsersFilter: FC<UsersFilterProps> = ({ filter, error, menus }) => {
	const presets = [
		{ query: userFilterQuery.active, name: "Active users" },
		...(menus?.aiSeat
			? [{ query: userFilterQuery.aiAddon, name: "AI add-on users" }]
			: []),
		{ query: userFilterQuery.serviceAccount, name: "Service accounts" },
		{ query: userFilterQuery.all, name: "All users" },
	];

	return (
		<Filter
			presets={presets}
			learnMoreLink={docs("/admin/users#user-filtering")}
			learnMoreLabel2="User status"
			learnMoreLink2={docs("/admin/users#user-status")}
			isLoading={
				menus?.status?.isInitializing || menus?.aiSeat?.isInitializing || false
			}
			filter={filter}
			error={error}
			options={
				<>
					{menus?.status && <StatusMenu {...menus.status} />}
					{menus?.aiSeat && <AISeatMenu {...menus.aiSeat} />}
				</>
			}
			optionsSkeleton={
				<>
					{menus?.status && <MenuSkeleton />}
					{menus?.aiSeat && <MenuSkeleton />}
				</>
			}
		/>
	);
};

const StatusMenu = (menu: StatusFilterMenu) => {
	return (
		<SelectFilter
			label="Select a status"
			placeholder="All statuses"
			options={menu.searchOptions}
			onSelect={menu.selectOption}
			selectedOption={menu.selectedOption ?? undefined}
		/>
	);
};

const AISeatMenu = (menu: AISeatFilterMenu) => {
	return (
		<SelectFilter
			label="Select AI add-on status"
			placeholder="All AI add-ons"
			options={menu.searchOptions}
			onSelect={menu.selectOption}
			selectedOption={menu.selectedOption ?? undefined}
		/>
	);
};
