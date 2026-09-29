import { DownloadIcon } from "lucide-react";
import { MaxAISpendPeriodDays, type Organization } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { DateTimeRangePicker } from "#/components/DateTimeRangePicker/DateTimeRangePicker";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { Spinner } from "#/components/Spinner/Spinner";
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
	onExportCSV: () => void;
	isExportingCSV: boolean;
};

export const SpendFilters: React.FC<SpendFiltersProps> = ({
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
	onExportCSV,
	isExportingCSV,
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
			<Button
				variant="outline"
				size="lg"
				className="shrink-0"
				onClick={onExportCSV}
				disabled={isExportingCSV}
			>
				<Spinner loading={isExportingCSV}>
					<DownloadIcon />
				</Spinner>
				Export CSV
			</Button>
		</div>
	);
};
