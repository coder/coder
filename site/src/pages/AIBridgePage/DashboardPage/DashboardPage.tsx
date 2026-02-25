import { useAuthenticated } from "hooks";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "modules/permissions/RequirePermission";
import { PaywallAIGovernance } from "components/Paywall/PaywallAIGovernance";
import type { FC } from "react";
import { pageTitle } from "utils/page";

// TODO: Replace placeholder values with real data from the AI Bridge
// dashboard API endpoint once it is implemented.

interface StatCardProps {
	label: string;
	value: string;
	subtitle: string;
}

const StatCard: FC<StatCardProps> = ({ label, value, subtitle }) => {
	return (
		<div className="flex flex-col gap-1 rounded-lg border border-border p-6">
			<span className="text-xs font-medium text-content-secondary uppercase tracking-wide">
				{label}
			</span>
			<span className="text-4xl font-bold text-content-primary">{value}</span>
			<span className="text-xs text-content-secondary">{subtitle}</span>
		</div>
	);
};

const DashboardPage: FC = () => {
	const feats = useFeatureVisibility();
	const { permissions } = useAuthenticated();

	const isEntitled = Boolean(feats.aibridge);
	const hasPermission = permissions.viewAnyAIBridgeInterception;

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>{pageTitle("Dashboard", "AI Bridge")}</title>

			{!isEntitled ? (
				<PaywallAIGovernance />
			) : (
				<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
					<StatCard label="Active Developers" value="—" subtitle="This week" />
					<StatCard label="Total Sessions" value="—" subtitle="All time" />
					<StatCard label="Total Commits" value="—" subtitle="All time" />
					<StatCard label="Total Tokens" value="—" subtitle="All time" />
				</div>
			)}
		</RequirePermission>
	);
};

export default DashboardPage;
