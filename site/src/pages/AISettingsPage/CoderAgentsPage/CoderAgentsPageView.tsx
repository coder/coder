import { InfoIcon } from "lucide-react";
import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import { Link as RouterLink } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { Link } from "#/components/Link/Link";
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
	hasAgentRuntimeLicense?: boolean;
	agentRuntimeHoursFeature?: TypesGen.Feature;
	isAgentRuntimeUsageLoading: boolean;
	isAgentRuntimeUsageUnavailable: boolean;
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
	isLoading: boolean;
	isUnavailable: boolean;
};

const CoderAgentsUsage: FC<CoderAgentsUsageProps> = ({
	hasAgentRuntimeLicense,
	feature,
	isLoading,
	isUnavailable,
}) => {
	const usedHours = formatAgentHours(feature?.actual_ms);
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

			{isLoading ? (
				<div className="mt-5 flex items-center gap-2 text-sm text-content-secondary">
					<Spinner size="sm" loading label="Loading Agent Time usage" />
					<span>Loading Agent Time usage...</span>
				</div>
			) : isUnavailable || hasAgentRuntimeLicense === undefined ? (
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
	hasAgentRuntimeLicense,
	agentRuntimeHoursFeature,
	isAgentRuntimeUsageLoading,
	isAgentRuntimeUsageUnavailable,
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
}) => (
	<div className="flex max-w-4xl flex-col gap-8">
		<SettingsHeader>
			<SettingsHeaderTitle>Coder Agents</SettingsHeaderTitle>
			<SettingsHeaderDescription>
				Configure deployment-wide Coder Agents capabilities. Model defaults are
				configured per organization in{" "}
				<RouterLink to="/ai/settings/models/defaults">
					Defaults & overrides
				</RouterLink>
				.
			</SettingsHeaderDescription>
		</SettingsHeader>
		<CoderAgentsUsage
			hasAgentRuntimeLicense={hasAgentRuntimeLicense}
			feature={agentRuntimeHoursFeature}
			isLoading={isAgentRuntimeUsageLoading}
			isUnavailable={isAgentRuntimeUsageUnavailable}
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
	</div>
);
