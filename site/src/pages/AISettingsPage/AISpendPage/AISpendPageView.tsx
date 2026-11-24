import type * as TypesGen from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { ExperimentalBadge } from "#/components/Badge/PresetBadges";
import { Loader } from "#/components/Loader/Loader";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	Tabs,
	TabsContent,
	TabsList,
	TabsTrigger,
} from "#/components/Tabs/Tabs";
import { PremiumPaywallAIGovernance } from "#/modules/paywall/PremiumPaywallAIGovernance";
import { AIBridgeSetupAlert } from "#/pages/AIBridgePage/AIBridgeSetupAlert";
import { DetailedReportTab } from "./DetailedReportTab";
import type { DetailedReportQuery } from "./DetailedReportTable";
import { OverviewTab } from "./OverviewTab";
import type { SpendDays } from "./spendDays";

export type AISpendTab = "overview" | "detailed-report";

type AISpendPageViewProps = {
	isEntitled: boolean;
	isEnabled: boolean;
	now: Date | undefined;
	organizations: readonly TypesGen.Organization[];
	organization: TypesGen.Organization | undefined;
	onOrganizationChange: (organization: TypesGen.Organization) => void;
	tab: AISpendTab;
	onTabChange: (tab: string) => void;
	isOrganizationsLoading: boolean;
	organizationsError: Error | null;
	days: SpendDays;
	onDaysChange: (days: SpendDays) => void;
	minDay: string | undefined;
	reportFilter: string;
	onReportFilterChange: (filter: string) => void;
	reportQuery: DetailedReportQuery;
	hasRunReport: boolean;
	isReportOutdated: boolean;
	onRunReport: () => void;
	onCancelReport: () => void;
	canExportReport: boolean;
	isExportingReport: boolean;
	onExportReport: () => void;
	onOpenDetailedReport: (username?: string) => void;
	overview: TypesGen.OrganizationAISpendOverview | undefined;
	overviewError: Error | null;
	unpricedModels: TypesGen.UnpricedAIModels | undefined;
	isUnpricedModelsLoading: boolean;
	topSpenderDetails: readonly TypesGen.OrganizationAISpendUser[] | undefined;
	topSpenderDetailsError: Error | null;
	canViewSessions: boolean;
};

export const AISpendPageView: React.FC<AISpendPageViewProps> = ({
	isEntitled,
	isEnabled,
	now,
	organizations,
	organization,
	onOrganizationChange,
	tab,
	onTabChange,
	isOrganizationsLoading,
	organizationsError,
	days,
	onDaysChange,
	minDay,
	reportFilter,
	onReportFilterChange,
	reportQuery,
	hasRunReport,
	isReportOutdated,
	onRunReport,
	onCancelReport,
	canExportReport,
	isExportingReport,
	onExportReport,
	onOpenDetailedReport,
	overview,
	overviewError,
	unpricedModels,
	isUnpricedModelsLoading,
	topSpenderDetails,
	topSpenderDetailsError,
	canViewSessions,
}) => {
	if (!isEntitled) {
		return (
			<PremiumPaywallAIGovernance variant="governance" source="ai_governance" />
		);
	}

	if (!isEnabled) {
		return <AIBridgeSetupAlert />;
	}

	if (isOrganizationsLoading) {
		return <Loader />;
	}

	if (organizations.length === 0 && organizationsError !== null) {
		return <ErrorAlert error={organizationsError} />;
	}

	return (
		<div className="flex max-w-275 flex-col gap-4">
			<SettingsHeader
				className="pb-2"
				actions={
					organizations.length > 0 &&
					(organization === undefined || organizations.length > 1) && (
						<OrganizationAutocomplete
							value={organization ?? null}
							ariaLabel={
								organization
									? `Organization ${getOrganizationLabel(organization, organizations)}`
									: "Organization"
							}
							options={organizations}
							required
							triggerClassName="w-60"
							optionsTabbable
							onChange={(next) => {
								if (next) {
									onOrganizationChange(next);
								}
							}}
						/>
					)
				}
			>
				<SettingsHeaderTitle tooltip={<ExperimentalBadge />}>
					AI spend
				</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Monitor total and per-user AI Gateway spend for the selected
					organization.
				</SettingsHeaderDescription>
			</SettingsHeader>
			{organizationsError !== null && <ErrorAlert error={organizationsError} />}
			{organization === undefined ? (
				<Alert severity="warning">
					This organization is unavailable, or you don't have access to it.
				</Alert>
			) : (
				<Tabs
					value={tab}
					onValueChange={onTabChange}
					className="flex flex-col gap-6"
				>
					<TabsList>
						<TabsTrigger value="overview">Overview</TabsTrigger>
						<TabsTrigger value="detailed-report">Detailed report</TabsTrigger>
					</TabsList>
					<TabsContent value="overview">
						<OverviewTab
							now={now}
							days={days}
							onDaysChange={onDaysChange}
							minDay={minDay}
							overview={overview}
							overviewError={overviewError}
							unpricedModels={unpricedModels}
							isUnpricedModelsLoading={isUnpricedModelsLoading}
							topSpenderDetails={topSpenderDetails}
							topSpenderDetailsError={topSpenderDetailsError}
							organizationName={getOrganizationLabel(
								organization,
								organizations,
							)}
							canViewSessions={canViewSessions}
							onOpenDetailedReport={onOpenDetailedReport}
						/>
					</TabsContent>
					<TabsContent value="detailed-report">
						<DetailedReportTab
							organizationId={organization.id}
							organizationName={getOrganizationLabel(
								organization,
								organizations,
							)}
							now={now}
							days={days}
							onDaysChange={onDaysChange}
							minDay={minDay}
							filter={reportFilter}
							onFilterChange={onReportFilterChange}
							canViewSessions={canViewSessions}
							reportQuery={reportQuery}
							hasRunReport={hasRunReport}
							isReportOutdated={isReportOutdated}
							onRunReport={onRunReport}
							onCancelReport={onCancelReport}
							canExportReport={canExportReport}
							isExportingReport={isExportingReport}
							onExportReport={onExportReport}
						/>
					</TabsContent>
				</Tabs>
			)}
		</div>
	);
};
