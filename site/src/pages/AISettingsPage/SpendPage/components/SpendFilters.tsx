import { CalendarIcon } from "lucide-react";
import type { FC } from "react";
import { MaxAISpendPeriodDays, type Organization } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DateRangePicker,
	type DateRangeValue,
} from "#/components/DateRangePicker/DateRangePicker";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import {
	ClientFilter,
	type ClientFilterMenu,
} from "#/pages/AIBridgePage/filters/ClientFilter";
import {
	ModelFilter,
	type ModelFilterMenu,
} from "#/pages/AIBridgePage/filters/ModelFilter";
import {
	ProviderFilter,
	type ProviderFilterMenu,
} from "#/pages/AIBridgePage/filters/ProviderFilter";

const FILTER_WIDTH = 150;

export type SpendFilterMenus = {
	provider: ProviderFilterMenu;
	model: ModelFilterMenu;
	client: ClientFilterMenu;
};

type SpendFiltersProps = {
	organizations: readonly Organization[];
	organization: Organization;
	onOrganizationChange: (organization: Organization) => void;
	menus: SpendFilterMenus | undefined;
	now: Date | undefined;
	dateRange: DateRangeValue | undefined;
	minDate: Date | undefined;
	isReportLoading: boolean;
	onDateRangeChange: (value: DateRangeValue) => void;
};

export const SpendFilters: FC<SpendFiltersProps> = ({
	organizations,
	organization,
	onOrganizationChange,
	menus,
	now,
	dateRange,
	minDate,
	isReportLoading,
	onDateRangeChange,
}) => {
	return (
		<div className="flex flex-wrap gap-2">
			{organizations.length > 1 && (
				<OrganizationAutocomplete
					value={organization}
					ariaLabel={`Organization ${getOrganizationLabel(
						organization,
						organizations,
					)}`}
					options={organizations}
					triggerClassName="basis-[150px] grow"
					optionsTabbable
					onChange={(next) => {
						if (next) {
							onOrganizationChange(next);
						}
					}}
				/>
			)}
			{menus && (
				<>
					<ProviderFilter menu={menus.provider} width={FILTER_WIDTH} />
					<ModelFilter menu={menus.model} width={FILTER_WIDTH} />
					<ClientFilter menu={menus.client} width={FILTER_WIDTH} />
				</>
			)}
			{dateRange ? (
				<DateRangePicker
					now={now}
					value={dateRange}
					onChange={onDateRangeChange}
					maxDays={MaxAISpendPeriodDays}
					minDate={minDate}
					disabled={isReportLoading}
					size="lg"
				/>
			) : (
				<Button variant="outline" size="lg" disabled>
					<CalendarIcon className="size-4 text-content-secondary" />
					Current budget period
				</Button>
			)}
		</div>
	);
};
