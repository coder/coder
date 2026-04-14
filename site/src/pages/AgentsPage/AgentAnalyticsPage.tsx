import dayjs, { type Dayjs } from "dayjs";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { chatCostSummary } from "#/api/queries/chats";
import { deploymentConfig } from "#/api/queries/deployment";
import { DataProtectionBanner } from "#/components/DataProtectionBanner";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { useAuthContext } from "#/contexts/auth/AuthProvider";
import { AgentAnalyticsPageView } from "./AgentAnalyticsPageView";
import { AgentPageHeader } from "./components/AgentPageHeader";

const createDateRange = (now?: Dayjs) => {
	const end = now ?? dayjs();
	const start = end.subtract(30, "day");
	return {
		startDate: start.toISOString(),
		endDate: end.toISOString(),
		rangeLabel: `${start.format("MMM D")} – ${end.format("MMM D, YYYY")}`,
	};
};

interface AgentAnalyticsPageProps {
	/** Override the current time for deterministic storybook snapshots. */
	now?: Dayjs;
}

const AgentAnalyticsPage: FC<AgentAnalyticsPageProps> = ({ now }) => {
	const { user } = useAuthContext();
	const [anchor] = useState<Dayjs>(() => dayjs());
	const dateRange = createDateRange(now ?? anchor);

	const configQuery = useQuery(deploymentConfig());
	const dataProtectionEnabled =
		configQuery.data?.config?.data_protection?.enabled;

	const summaryQuery = useQuery({
		...chatCostSummary(user?.id ?? "me", {
			start_date: dateRange.startDate,
			end_date: dateRange.endDate,
		}),
		enabled: Boolean(user?.id),
	});

	return (
		<ScrollArea className="min-h-0 flex-1" viewportClassName="[&>div]:!block">
			<AgentPageHeader mobileBack={{ to: "/agents", label: "Agents" }} />
			<DataProtectionBanner dataProtectionEnabled={dataProtectionEnabled} />
			<AgentAnalyticsPageView
				summary={summaryQuery.data}
				isLoading={summaryQuery.isLoading}
				error={summaryQuery.error}
				onRetry={() => void summaryQuery.refetch()}
				rangeLabel={dateRange.rangeLabel}
			/>
		</ScrollArea>
	);
};

export default AgentAnalyticsPage;
