import { PlusIcon, RotateCwIcon } from "lucide-react";
import Confetti from "react-confetti";
import { Link as RouterLink } from "react-router";
import type { GetLicensesResponse } from "#/api/api";
import type { Feature, UserStatusChangeCount } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Link } from "#/components/Link/Link";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useWindowSize } from "#/hooks/useWindowSize";
import { CONTACT_SALES_LINK } from "#/modules/licenses/trialLicense";
import { useTheme } from "#/theme/context";
import { AIGovernanceUsersConsumption } from "./AIGovernanceUsersConsumptionChart";
import { LicenseCard } from "./LicenseCard";
import { LicenseSeatConsumptionChart } from "./LicenseSeatConsumptionChart";
import { SeatUsageBarCard } from "./SeatUsageBarCard";
import { TotalAgentHoursCard } from "./TotalAgentHoursCard";

type Props = {
	showConfetti: boolean;
	isLoading: boolean;
	hasUserLimitEntitlementData: boolean;
	userLimitActual?: number;
	userLimitLimit?: number;
	licenses?: GetLicensesResponse[];
	licensesError?: unknown;
	isRetryingLicenses?: boolean;
	onRetryLicenses?: () => void;
	canViewLicenses: boolean;
	isRemovingLicense: boolean;
	isRefreshing: boolean;
	removeLicense: (licenseId: number) => void;
	refreshEntitlements: () => void;
	activeUsers: UserStatusChangeCount[] | undefined;
	aiGovernanceUserFeature?: Feature;
	agentRuntimeHoursFeature?: Feature;
};

const LicensesSettingsPageView: React.FC<Props> = ({
	showConfetti,
	isLoading,
	hasUserLimitEntitlementData,
	userLimitActual,
	userLimitLimit,
	licenses,
	licensesError,
	isRetryingLicenses = false,
	onRetryLicenses,
	canViewLicenses,
	isRemovingLicense,
	isRefreshing,
	removeLicense,
	refreshEntitlements,
	activeUsers,
	aiGovernanceUserFeature,
	agentRuntimeHoursFeature,
}) => {
	const theme = useTheme();
	const { width, height } = useWindowSize();

	return (
		<>
			<Confetti
				// For some reason this overflows the window and adds scrollbars if we don't subtract here.
				width={width - 1}
				height={height - 1}
				numberOfPieces={showConfetti ? 200 : 0}
				colors={[theme.palette.primary.main, theme.palette.secondary.main]}
			/>

			<div className="flex flex-row gap-4 items-baseline justify-between">
				<SettingsHeader>
					<SettingsHeaderTitle>Licenses</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Manage licenses to unlock Premium features.
					</SettingsHeaderDescription>
				</SettingsHeader>

				<div className="flex flex-row gap-4">
					<Button variant="outline" asChild>
						<RouterLink to="/deployment/licenses/add">
							<PlusIcon />
							Add a license
						</RouterLink>
					</Button>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								disabled={isRefreshing}
								onClick={refreshEntitlements}
								variant="outline"
							>
								<Spinner loading={isRefreshing}>
									<RotateCwIcon />
								</Spinner>
								Refresh
							</Button>
						</TooltipTrigger>
						<TooltipContent side="bottom" className="max-w-xs">
							Refresh license entitlements. This is done automatically every 10
							minutes.
						</TooltipContent>
					</Tooltip>
				</div>
			</div>

			<div className="flex flex-col gap-4">
				{isLoading && <Skeleton height={78} />}

				{!isLoading && Boolean(licensesError) && (
					<ErrorAlert
						error={licensesError}
						actions={
							onRetryLicenses && (
								<Button
									variant="outline"
									size="sm"
									type="button"
									disabled={isRetryingLicenses}
									onClick={onRetryLicenses}
								>
									<Spinner loading={isRetryingLicenses} />
									Retry
								</Button>
							)
						}
					/>
				)}

				{!isLoading && licenses && licenses?.length > 0 && (
					<div className="flex flex-col gap-8 licenses">
						{[...(licenses ?? [])]
							?.sort(
								(a, b) =>
									new Date(b.claims.license_expires).valueOf() -
									new Date(a.claims.license_expires).valueOf(),
							)
							.map((license) => (
								<LicenseCard
									key={license.id}
									license={license}
									userLimitActual={userLimitActual}
									userLimitLimit={userLimitLimit}
									aiGovernanceUserFeature={aiGovernanceUserFeature}
									agentRuntimeHoursFeature={agentRuntimeHoursFeature}
									isRemoving={isRemovingLicense}
									onRemove={removeLicense}
								/>
							))}
					</div>
				)}

				{!isLoading && !canViewLicenses && (
					<EmptyState
						message="You don't have permission to view licenses"
						description="Contact your deployment administrator for access."
					/>
				)}

				{!isLoading && canViewLicenses && licenses?.length === 0 && (
					<EmptyState
						className="rounded-lg border border-solid border-border"
						message="No licenses yet"
						description={
							<>
								Add a license to unlock high availability, RBAC, quotas, and
								more.{" "}
								<Link
									href={CONTACT_SALES_LINK}
									className="m-0 p-0 text-sm"
									showExternalIcon={false}
								>
									Contact sales
								</Link>{" "}
								or{" "}
								<RouterLink
									to="/deployment/premium"
									className="m-0 p-0 text-content-link"
								>
									request a trial license
								</RouterLink>{" "}
								to get started.
							</>
						}
						cta={
							<Button asChild>
								<RouterLink to="/deployment/licenses/add">
									<PlusIcon />
									Add a license
								</RouterLink>
							</Button>
						}
					/>
				)}

				{licenses && licenses.length > 0 && (
					<>
						<LicenseSeatConsumptionChart
							limit={userLimitLimit}
							data={activeUsers?.map((i) => ({
								date: i.date,
								users: i.count,
								limit: 80,
							}))}
						/>

						<div className="grid grid-cols-1 gap-4 md:grid-cols-2">
							{hasUserLimitEntitlementData && (
								<SeatUsageBarCard
									title="Seat usage"
									actual={userLimitActual}
									limit={userLimitLimit}
									allowUnlimited
								/>
							)}
							<AIGovernanceUsersConsumption
								aiGovernanceUserFeature={aiGovernanceUserFeature}
								licenses={licenses}
							/>
						</div>

						<TotalAgentHoursCard feature={agentRuntimeHoursFeature} />
					</>
				)}
			</div>
		</>
	);
};

export default LicensesSettingsPageView;
