import dayjs from "dayjs";
import type {
	AISpendPeriodWindow,
	OrganizationAISpendDetailsFilter,
} from "#/api/typesGenerated";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";

export const spendOrganizationCheck = {
	object: { resource_type: "group_member" },
	action: "read",
} as const;

export const spendOrganizationSearchParam = "organization";
export const spendStartDateSearchParam = "startDate";
export const spendEndDateSearchParam = "endDate";

export const dateRangeFromSearchParams = (
	searchParams: URLSearchParams,
): DateRangeValue | undefined => {
	const startDate = searchParams.get(spendStartDateSearchParam);
	const endDate = searchParams.get(spendEndDateSearchParam);
	if (!startDate || !endDate) {
		return undefined;
	}

	const parsedStartDate = new Date(startDate);
	const parsedEndDate = new Date(endDate);
	if (
		Number.isNaN(parsedStartDate.getTime()) ||
		Number.isNaN(parsedEndDate.getTime()) ||
		parsedStartDate >= parsedEndDate
	) {
		return undefined;
	}

	return { startDate: parsedStartDate, endDate: parsedEndDate };
};

export const spendDetailsFilter = (
	searchParams: URLSearchParams,
): OrganizationAISpendDetailsFilter => {
	const range = dateRangeFromSearchParams(searchParams);
	return {
		...(range && {
			period_start: range.startDate.toISOString(),
			period_end: range.endDate.toISOString(),
		}),
		user_id: searchParams.get("user_id") || undefined,
		group_id: searchParams.get("group_id") || undefined,
		provider_name: searchParams.get("provider_name") || undefined,
		model: searchParams.get("model") || undefined,
	};
};

export const retentionMinDate = (
	retentionStart: string | undefined,
	now: Date,
): Date | undefined => {
	if (!retentionStart) {
		return undefined;
	}

	const retention = dayjs(retentionStart);
	const minimum = retention.isSame(retention.startOf("day"))
		? retention
		: retention.add(1, "day").startOf("day");
	const today = dayjs(now).startOf("day");
	return minimum.isAfter(today) ? today.toDate() : minimum.toDate();
};

// The picker trigger displays dates, while API end bounds are exclusive.
export const displayDateRange = (range: DateRangeValue): DateRangeValue => ({
	startDate: range.startDate,
	endDate: dayjs(range.endDate).isSame(dayjs(range.endDate).startOf("day"))
		? new Date(range.endDate.getTime() - 1)
		: range.endDate,
});

export const appliedWindowToDateRange = (
	window: AISpendPeriodWindow,
	now: Date,
): DateRangeValue => {
	const start = new Date(window.period_start);
	const end = new Date(
		Math.min(new Date(window.period_end).getTime(), now.getTime()),
	);
	return displayDateRange({
		startDate: start,
		endDate: end > start ? end : new Date(start.getTime() + 1),
	});
};
