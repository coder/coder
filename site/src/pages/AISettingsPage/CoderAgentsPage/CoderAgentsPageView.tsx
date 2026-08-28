import { InfoIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import type { UseMutateFunction } from "react-query";
import { Link as RouterLink } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
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
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { AdvisorSettings } from "#/pages/AgentsPage/components/AdvisorSettings";
import { VirtualDesktopSettings } from "#/pages/AgentsPage/components/VirtualDesktopSettings";
import { docs } from "#/utils/docs";
import {
	AdminPersonalModelOverridesSettings,
	type SavePersonalModelOverridesAdminSetting,
} from "./components/AdminPersonalModelOverridesSettings";
import type { MutationCallbacks } from "./components/SubagentModelOverrideSettings";

export interface CoderAgentsPageViewProps {
	organization?: TypesGen.Organization;
	organizations: readonly TypesGen.Organization[];
	onSelectOrganization: (organization: TypesGen.Organization) => void;
	organizationAccessError?: unknown;
	organizationPermissionsError?: unknown;
	requestedOrganizationDenied: boolean;
	isOrganizationAccessLoading: boolean;
	organizationSettings?: ReactNode;
	canEditDeploymentConfig: boolean;
	hasAgentRuntimeLicense?: boolean;
	agentRuntimeHoursFeature?: TypesGen.Feature;
	agentRuntimeTotalMs?: number;
	isAgentRuntimeUsageLoading: boolean;
	isAgentRuntimeUsageUnavailable: boolean;
	agentRuntimeUsageError?: unknown;
	onRetryAgentRuntimeUsage: () => void;
	isRetryingAgentRuntimeUsage: boolean;
	adminOverridesData?: TypesGen.ChatPersonalModelOverridesAdminSettings;
	adminOverridesError?: unknown;
	onRetryAdminOverrides?: () => void;
	isRetryingAdminOverrides?: boolean;
	onSaveAdminOverrides: SavePersonalModelOverridesAdminSetting;
	isSavingAdminOverrides: boolean;
	isSaveAdminOverridesError: boolean;
	showAdvisorSettings: boolean;
	advisorConfigData: TypesGen.AdvisorConfig | undefined;
	isAdvisorConfigLoading: boolean;
	isAdvisorConfigFetching: boolean;
	isAdvisorConfigLoadError: boolean;
	onSaveAdvisorConfig: (
		req: TypesGen.UpdateAdvisorConfigRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingAdvisorConfig: boolean;
	isSaveAdvisorConfigError: boolean;
	saveAdvisorConfigError: unknown;
	showVirtualDesktopSettings: boolean;
	computerUseProviderData: TypesGen.ChatComputerUseProviderResponse | undefined;
	isLoadingComputerUseProvider: boolean;
	onSaveComputerUseProvider: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatComputerUseProviderRequest,
		unknown
	>;
	isSavingComputerUseProvider: boolean;
	computerUseProviderSaveError: Error | null;
}

const maxConcurrentAgents = 5;
const concurrentAgentsTooltip =
	"Number of agents that can run at the same time.";
const concurrentAgentsHardLimitTooltip = `${concurrentAgentsTooltip} You've reached your limit: concurrent chats are now capped at ${maxConcurrentAgents} (down from unlimited).`;

const formatAgentHours = (actualMs: number | undefined): string => {
	if (actualMs === undefined || !Number.isFinite(actualMs)) {
		return "N/A";
	}
	const hours = Math.floor(Math.max(actualMs, 0) / 360_000) / 10;
	return hours.toLocaleString("en-US", {
		minimumFractionDigits: 1,
		maximumFractionDigits: 1,
	});
};

type CoderAgentsUsageProps = {
	hasAgentRuntimeLicense?: boolean;
	feature?: TypesGen.Feature;
	totalRuntimeMs?: number;
	isLoading: boolean;
	isUnavailable: boolean;
	error?: unknown;
	onRetry: () => void;
	isRetrying: boolean;
};

const CoderAgentsUsage: FC<CoderAgentsUsageProps> = ({
	hasAgentRuntimeLicense,
	feature,
	totalRuntimeMs,
	isLoading,
	isUnavailable,
	error,
	onRetry,
	isRetrying,
}) => {
	const usedHours = formatAgentHours(totalRuntimeMs);
	const hardLimitReached =
		feature?.enabled === true &&
		feature.hard_limit !== undefined &&
		feature.actual !== undefined &&
		feature.actual >= feature.hard_limit;
	const concurrentAgents =
		hasAgentRuntimeLicense && feature?.enabled && !hardLimitReached
			? "Unlimited"
			: maxConcurrentAgents.toLocaleString("en-US");
	const allocation =
		feature?.limit === undefined
			? "Unlimited"
			: Number.isFinite(feature.limit)
				? feature.limit.toLocaleString("en-US")
				: "N/A";
	const hasUsageData =
		hasAgentRuntimeLicense !== undefined && totalRuntimeMs !== undefined;
	const hasLoadError = error != null;

	return (
		<section
			aria-label="Coder Agents usage"
			className="rounded-lg border border-solid border-border px-6 py-5"
		>
			<div className="flex items-center justify-between gap-4">
				<div>
					<h2 className="m-0 text-base font-medium">Usage</h2>
					<Link asChild showExternalIcon={false} size="lg" className="mt-1">
						<RouterLink
							to={
								hasAgentRuntimeLicense === false
									? "/deployment/premium"
									: "/deployment/licenses"
							}
						>
							{hasAgentRuntimeLicense === false
								? "Upgrade for unlimited concurrent chats"
								: "View license"}
						</RouterLink>
					</Link>
				</div>
			</div>

			{hasLoadError && (
				<div className="mt-5 flex flex-col gap-2">
					<ErrorAlert error={error} />
					<Button
						disabled={isRetrying}
						onClick={onRetry}
						size="sm"
						type="button"
						variant="outline"
						className="w-fit"
					>
						Retry
					</Button>
				</div>
			)}
			{hasLoadError && !hasUsageData ? null : isLoading ? (
				<div className="mt-5 flex items-center gap-2 text-sm text-content-secondary">
					<Spinner size="sm" loading label="Loading Agent Time usage" />
					<span>Loading Agent Time usage...</span>
				</div>
			) : isUnavailable || !hasUsageData ? (
				<p role="status" className="m-0 mt-5 text-sm text-content-secondary">
					Agent Time usage is unavailable.
				</p>
			) : (
				<>
					<dl className="m-0 mt-5 grid gap-4 sm:grid-cols-2">
						<div>
							<dt className="text-xs font-medium text-content-secondary">
								Agent hours used
							</dt>
							<dd className="m-0 mt-1 text-sm font-medium text-content-primary">
								{hasAgentRuntimeLicense
									? `${usedHours} / ${allocation} hours`
									: `${usedHours} hours`}
							</dd>
						</div>
						<div>
							<dt className="flex items-center gap-1 text-xs font-medium text-content-secondary">
								<span>Max concurrent agents</span>
								<Tooltip>
									<TooltipTrigger asChild>
										<button
											type="button"
											aria-label="Max concurrent agents information"
											className="m-0 inline-flex appearance-none border-0 bg-transparent p-0 text-content-secondary"
										>
											<InfoIcon className="size-3" />
										</button>
									</TooltipTrigger>
									<TooltipContent side="top" className="max-w-xs">
										{hardLimitReached
											? concurrentAgentsHardLimitTooltip
											: concurrentAgentsTooltip}
									</TooltipContent>
								</Tooltip>
							</dt>
							<dd className="m-0 mt-1 text-sm font-medium text-content-primary">
								{concurrentAgents}
							</dd>
						</div>
					</dl>
					<p className="m-0 mt-4 text-sm text-content-secondary">
						<Link href={docs("/ai-coder/agents/licensing-usage")}>
							View usage documentation
						</Link>
					</p>
				</>
			)}
		</section>
	);
};

export const CoderAgentsPageView: FC<CoderAgentsPageViewProps> = ({
	organization,
	organizations,
	onSelectOrganization,
	organizationAccessError,
	organizationPermissionsError,
	requestedOrganizationDenied,
	isOrganizationAccessLoading,
	organizationSettings,
	canEditDeploymentConfig,
	hasAgentRuntimeLicense,
	agentRuntimeHoursFeature,
	agentRuntimeTotalMs,
	isAgentRuntimeUsageLoading,
	isAgentRuntimeUsageUnavailable,
	agentRuntimeUsageError,
	onRetryAgentRuntimeUsage,
	isRetryingAgentRuntimeUsage,
	adminOverridesData,
	adminOverridesError,
	onRetryAdminOverrides,
	isRetryingAdminOverrides,
	onSaveAdminOverrides,
	isSavingAdminOverrides,
	isSaveAdminOverridesError,
	showAdvisorSettings,
	advisorConfigData,
	isAdvisorConfigLoading,
	isAdvisorConfigFetching,
	isAdvisorConfigLoadError,
	onSaveAdvisorConfig,
	isSavingAdvisorConfig,
	isSaveAdvisorConfigError,
	saveAdvisorConfigError,
	showVirtualDesktopSettings,
	computerUseProviderData,
	isLoadingComputerUseProvider,
	onSaveComputerUseProvider,
	isSavingComputerUseProvider,
	computerUseProviderSaveError,
}) => {
	return (
		<div className="flex max-w-4xl flex-col gap-10">
			<SettingsHeader>
				<SettingsHeaderTitle>Coder Agents</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Configure organization model choices and deployment-wide Coder Agents
					capabilities.
				</SettingsHeaderDescription>
			</SettingsHeader>

			{isOrganizationAccessLoading ? (
				<Loader label="Loading organization settings" />
			) : organization ? (
				<section
					aria-labelledby="organization-agent-settings"
					className="flex flex-col gap-6"
				>
					<div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
						<div>
							<h2
								id="organization-agent-settings"
								className="m-0 text-xl font-semibold"
							>
								Organization settings
							</h2>
							<p className="mt-1 mb-0 text-sm text-content-secondary">
								Choose model and reasoning defaults for each Coder Agents
								context.
							</p>
						</div>
						{organizations.length > 1 && (
							<OrganizationAutocomplete
								value={organization}
								ariaLabel={`Organization ${getOrganizationLabel(
									organization,
									organizations,
								)}`}
								options={organizations}
								triggerClassName="w-60"
								optionsTabbable
								onChange={(nextOrganization) => {
									if (nextOrganization) {
										onSelectOrganization(nextOrganization);
									}
								}}
							/>
						)}
					</div>
					{requestedOrganizationDenied && (
						<Alert severity="warning">
							The requested organization is not available. Showing settings for{" "}
							{organization.display_name || organization.name} instead.
						</Alert>
					)}
					{organizationAccessError != null && (
						<ErrorAlert error={organizationAccessError} />
					)}
					{organizationPermissionsError != null && (
						<ErrorAlert error={organizationPermissionsError} />
					)}
					{organizationSettings}
				</section>
			) : organizationAccessError != null ? (
				<ErrorAlert error={organizationAccessError} />
			) : null}

			{canEditDeploymentConfig && (
				<section
					aria-labelledby="deployment-agent-settings"
					className="flex flex-col gap-6"
				>
					<div>
						<h2
							id="deployment-agent-settings"
							className="m-0 text-xl font-semibold"
						>
							Deployment settings
						</h2>
						<p className="mt-1 mb-0 text-sm text-content-secondary">
							Configure Coder Agents capabilities that apply to every
							organization.
						</p>
					</div>
					<CoderAgentsUsage
						hasAgentRuntimeLicense={hasAgentRuntimeLicense}
						feature={agentRuntimeHoursFeature}
						totalRuntimeMs={agentRuntimeTotalMs}
						isLoading={isAgentRuntimeUsageLoading}
						isUnavailable={isAgentRuntimeUsageUnavailable}
						error={agentRuntimeUsageError}
						onRetry={onRetryAgentRuntimeUsage}
						isRetrying={isRetryingAgentRuntimeUsage}
					/>
					<div className="flex flex-col gap-6 rounded-lg border border-solid border-border px-6 py-7">
						<AdminPersonalModelOverridesSettings
							adminSettings={adminOverridesData}
							adminSettingsError={adminOverridesError}
							onRetryAdminSettings={onRetryAdminOverrides}
							isRetryingAdminSettings={isRetryingAdminOverrides}
							onSaveAdminSetting={onSaveAdminOverrides}
							isSavingAdminSetting={isSavingAdminOverrides}
							isSaveAdminSettingError={isSaveAdminOverridesError}
						/>
						{showVirtualDesktopSettings && (
							<VirtualDesktopSettings
								computerUseProviderData={computerUseProviderData}
								isLoadingComputerUseProvider={isLoadingComputerUseProvider}
								onSaveComputerUseProvider={onSaveComputerUseProvider}
								isSavingComputerUseProvider={isSavingComputerUseProvider}
								computerUseProviderSaveError={computerUseProviderSaveError}
							/>
						)}
						{showAdvisorSettings && (
							<AdvisorSettings
								advisorConfigData={advisorConfigData}
								isAdvisorConfigLoading={isAdvisorConfigLoading}
								isAdvisorConfigFetching={isAdvisorConfigFetching}
								isAdvisorConfigLoadError={isAdvisorConfigLoadError}
								onSaveAdvisorConfig={onSaveAdvisorConfig}
								isSavingAdvisorConfig={isSavingAdvisorConfig}
								isSaveAdvisorConfigError={isSaveAdvisorConfigError}
								saveAdvisorConfigError={saveAdvisorConfigError}
							/>
						)}
					</div>
				</section>
			)}
		</div>
	);
};
