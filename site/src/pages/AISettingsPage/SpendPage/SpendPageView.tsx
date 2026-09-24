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
import { type SpendFilterMenus, SpendFilters } from "./components/SpendFilters";
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
	filterMenus: SpendFilterMenus | undefined;
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
	filterMenus,
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
				menus={filterMenus}
				now={now}
				period={period}
				minDate={minDate}
				onPeriodChange={onPeriodChange}
			/>
			<SpendUsersTable reportQuery={reportQuery} />
		</>
	);
};
