import { DownloadIcon } from "lucide-react";
import type { FC } from "react";
import type { Organization } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";
import { Loader } from "#/components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { PremiumPaywallAIGovernance } from "#/modules/paywall/PremiumPaywallAIGovernance";
import { AIBridgeSetupAlert } from "#/pages/AIBridgePage/AIBridgeSetupAlert";
import {
	type SpendDetailsFilterMenus,
	SpendDetailsFilters,
} from "./components/SpendDetailsFilters";
import {
	type SpendDetailsQuery,
	SpendDetailsTable,
} from "./components/SpendDetailsTable";

interface SpendDetailsPageViewProps {
	isEntitled: boolean;
	isEnabled: boolean;
	organizations: readonly Organization[];
	organization: Organization | undefined;
	onOrganizationChange: (organization: Organization) => void;
	requestedOrganizationDenied: boolean;
	isOrganizationsLoading: boolean;
	organizationsError: unknown;
	dateRange: DateRangeValue | undefined;
	now: Date;
	minDate: Date | undefined;
	onDateRangeChange: (value: DateRangeValue) => void;
	menus: SpendDetailsFilterMenus;
	onExport: () => void;
	isExporting: boolean;
	canExport: boolean;
	exportError: unknown;
	query: SpendDetailsQuery;
}

export const SpendDetailsPageView: FC<SpendDetailsPageViewProps> = ({
	isEntitled,
	isEnabled,
	organizations,
	organization,
	onOrganizationChange,
	requestedOrganizationDenied,
	isOrganizationsLoading,
	organizationsError,
	dateRange,
	now,
	minDate,
	onDateRangeChange,
	menus,
	onExport,
	isExporting,
	canExport,
	exportError,
	query,
}) => {
	if (!isEntitled) {
		return (
			<PremiumPaywallAIGovernance variant="governance" source="ai_governance" />
		);
	}

	if (!isEnabled) {
		return <AIBridgeSetupAlert />;
	}

	return (
		<div className="flex max-w-[1400px] flex-col gap-4">
			<SettingsHeader
				actions={
					organization ? (
						<Button
							variant="outline"
							size="lg"
							onClick={onExport}
							disabled={!canExport || isExporting}
						>
							<DownloadIcon />
							Export CSV
						</Button>
					) : undefined
				}
			>
				<SettingsHeaderTitle>AI spend details</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Review AI Gateway spend and token usage by user, effective group,
					provider, and model.
				</SettingsHeaderDescription>
			</SettingsHeader>
			{isOrganizationsLoading ? (
				<Loader />
			) : requestedOrganizationDenied ? (
				<ErrorAlert
					error={
						new Error("You do not have access to the requested organization.")
					}
					showDebugDetail={false}
				/>
			) : organization === undefined ? (
				organizationsError ? (
					<ErrorAlert error={organizationsError} />
				) : (
					<p className="py-12 text-center text-content-secondary">
						You do not have access to any organization&apos;s AI spend.
					</p>
				)
			) : (
				<>
					{organizationsError && <ErrorAlert error={organizationsError} />}
					{exportError && <ErrorAlert error={exportError} />}
					<SpendDetailsFilters
						organizations={organizations}
						organization={organization}
						onOrganizationChange={onOrganizationChange}
						dateRange={dateRange}
						now={now}
						minDate={minDate}
						onDateRangeChange={onDateRangeChange}
						menus={menus}
					/>
					<SpendDetailsTable query={query} />
				</>
			)}
		</div>
	);
};
