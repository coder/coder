import { type FC, useState } from "react";
import type { AIGatewaySpendFilter } from "#/api/typesGenerated";
import {
	DateRangePicker,
	type DateRangeValue,
} from "#/components/DateRangePicker/DateRangePicker";
import { SearchField } from "#/components/SearchField/SearchField";
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

// Narrower than the SelectFilter default so the search field keeps most of
// the row, matching the sessions page.
const FILTER_WIDTH = 150;

// The URL keys match both the spend API query parameters and the sessions
// page filter keys, so a drill-in can hand its filters to the sessions link
// unchanged.
export type SpendDimensions = Pick<
	AIGatewaySpendFilter,
	"provider_name" | "client" | "model"
>;

export interface SpendFilterMenus {
	provider: ProviderFilterMenu;
	client: ClientFilterMenu;
	model: ModelFilterMenu;
}

interface SpendFiltersProps {
	menus: SpendFilterMenus;
	now?: Date;
	dateRange: DateRangeValue;
	onDateRangeChange: (value: DateRangeValue) => void;
	search?: { value: string; onChange: (value: string) => void };
}

export const SpendFilters: FC<SpendFiltersProps> = ({
	menus,
	now,
	dateRange,
	onDateRangeChange,
	search,
}) => {
	// The settings sidebar leaves this row less room than the full-width
	// sessions filter, so it measures its own width instead of the viewport and
	// keeps wrapping until the search field would stay usable on one line.
	return (
		<div className="@container">
			<div className="flex flex-wrap gap-2 @5xl:flex-nowrap">
				{search && <SpendSearchField {...search} />}
				<ProviderFilter menu={menus.provider} width={FILTER_WIDTH} />
				<ClientFilter menu={menus.client} width={FILTER_WIDTH} />
				<ModelFilter menu={menus.model} width={FILTER_WIDTH} />
				<DateRangePicker
					now={now}
					value={dateRange}
					onChange={onDateRangeChange}
					size="lg"
				/>
			</div>
		</div>
	);
};

// The URL owns the search, but an input controlled by the URL waits for the
// router re-render between keystrokes and drops characters from fast typists,
// so the field shows its own draft while it has focus.
const SpendSearchField: FC<{
	value: string;
	onChange: (value: string) => void;
}> = ({ value, onChange }) => {
	const [draft, setDraft] = useState(value);
	const [isEditing, setIsEditing] = useState(false);

	return (
		<SearchField
			className="w-full"
			value={isEditing ? draft : value}
			onFocus={() => {
				setDraft(value);
				setIsEditing(true);
			}}
			onBlur={() => setIsEditing(false)}
			onChange={(next) => {
				setDraft(next);
				onChange(next);
			}}
			placeholder="Search by name or username"
			aria-label="Search spend by name or username"
		/>
	);
};
