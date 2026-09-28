import type { FC } from "react";
import { MaxAISpendPeriodDays, type Organization } from "#/api/typesGenerated";
import { DateTimeRangePicker } from "#/components/DateTimeRangePicker/DateTimeRangePicker";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { spendQuickPresets } from "../spendPeriod";
import { spendFilterCategories } from "./spendFilterCategories";

type SpendFiltersProps = {
	organizations: readonly Organization[];
	organization: Organization;
	onOrganizationChange: (organization: Organization) => void;
	canFilterDimensions: boolean;
	filterQuery: string;
	onFilterQueryChange: (query: string) => void;
	filterError: string | undefined;
	now: Date | undefined;
	period: DateTimeRangeValue;
	minDate: Date | undefined;
	onPeriodChange: (value: DateTimeRangeValue) => void;
};

export const SpendFilters: FC<SpendFiltersProps> = ({
	organizations,
	organization,
	onOrganizationChange,
	canFilterDimensions,
	filterQuery,
	onFilterQueryChange,
	filterError,
	now,
	period,
	minDate,
	onPeriodChange,
}) => {
	return (
		<div className="flex flex-wrap items-start gap-2">
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
			{canFilterDimensions && (
				<div className="min-w-60 flex-1">
					<FilterCombobox
						value={filterQuery}
						onChange={onFilterQueryChange}
						categories={spendFilterCategories}
						placeholder="Filter by provider, client, or model…"
						errorMessage={filterError}
					/>
				</div>
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
