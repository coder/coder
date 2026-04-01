import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Spinner } from "#/components/Spinner/Spinner";
import { type PRInsightsTimeRange, PRInsightsView } from "./PRInsightsView";

interface InsightsContentProps {
	data: TypesGen.PRInsightsResponse | undefined;
	isLoading: boolean;
	error: unknown;
	timeRange: PRInsightsTimeRange;
	onTimeRangeChange: (range: PRInsightsTimeRange) => void;
}

export const InsightsContent: FC<InsightsContentProps> = ({
	data,
	isLoading,
	error,
	timeRange,
	onTimeRangeChange,
}) => {
	if (isLoading) {
		return (
			<div className="flex min-h-[400px] items-center justify-center">
				<Spinner size="lg" loading />
			</div>
		);
	}

	if (error) {
		return (
			<div className="flex min-h-[400px] items-center justify-center">
				<p className="text-sm text-content-secondary">
					Failed to load analytics data.
				</p>
			</div>
		);
	}

	if (!data) {
		return null;
	}

	return (
		<PRInsightsView
			data={data}
			timeRange={timeRange}
			onTimeRangeChange={onTimeRangeChange}
		/>
	);
};
