import type { FC } from "react";
import { MaxAISpendPeriodDays, type Organization } from "#/api/typesGenerated";
import { DateTimeRangePicker } from "#/components/DateTimeRangePicker/DateTimeRangePicker";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
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
import { spendQuickPresets } from "../spendPeriod";

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
	period: DateTimeRangeValue;
	minDate: Date | undefined;
	onPeriodChange: (value: DateTimeRangeValue) => void;
};

export const SpendFilters: FC<SpendFiltersProps> = ({
	organizations,
	organization,
	onOrganizationChange,
	menus,
	now,
	period,
	minDate,
	onPeriodChange,
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
			<DateTimeRangePicker
				now={now}
				value={period}
				onChange={onPeriodChange}
				presets={spendQuickPresets}
				maxDays={MaxAISpendPeriodDays}
				minDate={minDate}
				size="lg"
			/>
		</div>
	);
};
