import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import { OrganizationAutocomplete } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { PremiumPaywallAIGovernance } from "#/modules/paywall/PremiumPaywallAIGovernance";
import { AIBridgeSetupAlert } from "#/pages/AIBridgePage/AIBridgeSetupAlert";
import { SpendFilters } from "./components/SpendFilters";
import {
	type SpendReportQuery,
	SpendUsersTable,
} from "./components/SpendUsersTable";

type SpendPageViewProps = {
	isEntitled: boolean;
	isEnabled: boolean;
	now: Date | undefined;
	organizations: readonly TypesGen.Organization[];
	organization: TypesGen.Organization | undefined;
	onOrganizationChange: (organization: TypesGen.Organization) => void;
	isOrganizationsLoading: boolean;
	organizationsError: unknown;
	period: DateTimeRangeValue;
	minDate: Date | undefined;
	onPeriodChange: (value: DateTimeRangeValue) => void;
	filterQuery: string;
	onFilterQueryChange: (query: string) => void;
	showDimensionFilters: boolean;
	unknownUsername: string | undefined;
	userLookupError: unknown;
	reportQuery: SpendReportQuery;
};

export const SpendPageView: FC<SpendPageViewProps> = ({
	isEntitled,
	isEnabled,
	...contentProps
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
		<div className="flex max-w-[1100px] flex-col gap-4">
			<SettingsHeader>
				<SettingsHeaderTitle>User spend</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Monitor total and per-user AI Gateway spend for the selected
					organization.
				</SettingsHeaderDescription>
			</SettingsHeader>
			<SpendPageContent {...contentProps} />
		</div>
	);
};

type SpendPageContentProps = Omit<
	SpendPageViewProps,
	"isEntitled" | "isEnabled"
>;

const SpendPageContent: FC<SpendPageContentProps> = ({
	now,
	organizations,
	organization,
	onOrganizationChange,
	isOrganizationsLoading,
	organizationsError,
	period,
	minDate,
	onPeriodChange,
	filterQuery,
	onFilterQueryChange,
	showDimensionFilters,
	unknownUsername,
	userLookupError,
	reportQuery,
}) => {
	if (isOrganizationsLoading) {
		return <Loader />;
	}

	if (organizations.length === 0) {
		return organizationsError != null ? (
			<ErrorAlert error={organizationsError} />
		) : (
			<EmptyState
				isCompact
				message="You don't have access to any organization's AI spend."
			/>
		);
	}

	const refetchErrorAlert = organizationsError != null && (
		<ErrorAlert error={organizationsError} />
	);

	if (organization === undefined) {
		return (
			<>
				{refetchErrorAlert}
				<OrganizationAutocomplete
					value={null}
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
				<Alert severity="warning">
					This organization is unavailable, or you don't have access to it.
				</Alert>
			</>
		);
	}

	return (
		<>
			{refetchErrorAlert}
			<SpendFilters
				organizations={organizations}
				organization={organization}
				onOrganizationChange={onOrganizationChange}
				filterQuery={filterQuery}
				onFilterQueryChange={onFilterQueryChange}
				showDimensionFilters={showDimensionFilters}
				now={now}
				period={period}
				minDate={minDate}
				onPeriodChange={onPeriodChange}
			/>
			<SpendReport
				unknownUsername={unknownUsername}
				userLookupError={userLookupError}
				reportQuery={reportQuery}
			/>
		</>
	);
};

type SpendReportProps = Pick<
	SpendPageViewProps,
	"unknownUsername" | "userLookupError" | "reportQuery"
>;

const SpendReport: FC<SpendReportProps> = ({
	unknownUsername,
	userLookupError,
	reportQuery,
}) => {
	if (userLookupError != null) {
		return <ErrorAlert error={userLookupError} />;
	}
	if (unknownUsername !== undefined) {
		return (
			<Alert severity="warning">
				No member named {unknownUsername} in this organization.
			</Alert>
		);
	}
	return <SpendUsersTable reportQuery={reportQuery} />;
};
