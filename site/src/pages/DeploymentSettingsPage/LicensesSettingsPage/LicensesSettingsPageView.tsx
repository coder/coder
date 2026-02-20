import type { GetLicensesResponse } from "api/api";
import type { Feature, UserStatusChangeCount } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Link } from "components/Link/Link";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Skeleton } from "components/Skeleton/Skeleton";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { PlusIcon, RotateCwIcon } from "lucide-react";
import { LicenseSuccessDialog } from "modules/management/LicenseSuccessDialog/LicenseSuccessDialog";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { AIGovernanceUsersConsumption } from "./AIGovernanceUsersConsumptionChart";
import { LicenseCard } from "./LicenseCard";
import { LicenseSeatConsumptionChart } from "./LicenseSeatConsumptionChart";
import { ManagedAgentsConsumption } from "./ManagedAgentsConsumption";

type Props = {
	isSuccess: boolean;
	isLoading: boolean;
	licenseTier: string | null;
	userLimitActual?: number;
	userLimitLimit?: number;
	licenses?: GetLicensesResponse[];
	isRemovingLicense: boolean;
	isRefreshing: boolean;
	removeLicense: (licenseId: number) => void;
	refreshEntitlements: () => void;
	activeUsers: UserStatusChangeCount[] | undefined;
	managedAgentFeature?: Feature;
	aiGovernanceUserFeature?: Feature;
	onCloseSuccess: () => void;
};

const LicensesSettingsPageView: FC<Props> = ({
	isSuccess,
	isLoading,
	licenseTier,
	userLimitActual,
	userLimitLimit,
	licenses,
	isRemovingLicense,
	isRefreshing,
	removeLicense,
	refreshEntitlements,
	activeUsers,
	managedAgentFeature,
	aiGovernanceUserFeature,
	onCloseSuccess,
}) => {
	return (
		<>
			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<SettingsHeader>
					<SettingsHeaderTitle>Licenses</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Manage licenses to unlock Premium features.
					</SettingsHeaderDescription>
				</SettingsHeader>

				<Stack direction="row" spacing={2}>
					<Button variant="outline" asChild>
						<RouterLink className="px-0" to="/deployment/licenses/add">
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
				</Stack>
			</Stack>

			<div className="flex flex-col gap-4">
				{isLoading && <Skeleton className="h-[78px] rounded" />}

				{!isLoading && licenses && licenses?.length > 0 && (
					<Stack spacing={4} className="licenses">
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
									isRemoving={isRemovingLicense}
									onRemove={removeLicense}
								/>
							))}
					</Stack>
				)}

				{!isLoading && licenses === null && (
					<div className="min-h-[240px] flex items-center justify-center rounded-lg border border-solid border-border p-12">
						<Stack alignItems="center" spacing={1}>
							<Stack alignItems="center" spacing={0.5}>
								<span className="text-base">
									You don&apos;t have any licenses!
								</span>
								<span className="text-content-secondary text-center max-w-[464px] mt-2">
									You&apos;re missing out on high availability, RBAC, quotas,
									and much more. Contact{" "}
									<Link href="mailto:sales@coder.com">sales</Link> or{" "}
									<Link href="https://coder.com/trial">
										request a trial license
									</Link>{" "}
									to get started.
								</span>
							</Stack>
						</Stack>
					</div>
				)}

				{licenses && licenses.length > 0 && (
					<LicenseSeatConsumptionChart
						limit={userLimitLimit}
						data={activeUsers?.map((i) => ({
							date: i.date,
							users: i.count,
							limit: 80,
						}))}
					/>
				)}

				{licenses && licenses.length > 0 && (
					<>
						<ManagedAgentsConsumption
							managedAgentFeature={managedAgentFeature}
						/>
						<AIGovernanceUsersConsumption
							aiGovernanceUserFeature={aiGovernanceUserFeature}
						/>
					</>
				)}
			</div>

			<LicenseSuccessDialog
				open={isSuccess}
				onClose={onCloseSuccess}
				licenseTier={licenseTier}
			/>
		</>
	);
};

export default LicensesSettingsPageView;
