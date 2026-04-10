import {
	CheckCircleIcon,
	ChevronRightIcon,
	CircleIcon,
	PlusIcon,
} from "lucide-react";
import type { FC, ReactNode } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { cn } from "#/utils/cn";
import { SectionHeader } from "../SectionHeader";
import type { ProviderState } from "./ChatModelAdminPanel";
import { formatProviderConfigLabel } from "./modelProviderOptions";
import { ProviderForm } from "./ProviderForm";
import { ProviderIcon } from "./ProviderIcon";

const nilProviderConfigID = "00000000-0000-0000-0000-000000000000";

const clearProviderParams = (params: URLSearchParams) => {
	params.delete("provider");
	params.delete("configId");
	params.delete("newConfig");
	return params;
};

type ProviderView = { mode: "list" } | { mode: "detail"; provider: string };

interface ProvidersSectionProps {
	sectionLabel?: string;
	sectionDescription?: string;
	sectionBadge?: ReactNode;
	providerStates: readonly ProviderState[];
	providerConfigsUnavailable: boolean;
	isProviderMutationPending: boolean;
	onCreateProvider: (
		req: TypesGen.CreateChatProviderConfigRequest,
	) => Promise<unknown>;
	onUpdateProvider: (
		providerConfigId: string,
		req: TypesGen.UpdateChatProviderConfigRequest,
	) => Promise<unknown>;
	onDeleteProvider: (providerConfigId: string) => Promise<void>;
	onSelectedProviderChange: (provider: string) => void;
}

export const ProvidersSection: FC<ProvidersSectionProps> = ({
	sectionLabel,
	sectionDescription,
	sectionBadge,
	providerStates,
	providerConfigsUnavailable,
	isProviderMutationPending,
	onCreateProvider,
	onUpdateProvider,
	onDeleteProvider,
	onSelectedProviderChange,
}) => {
	const [searchParams, setSearchParams] = useSearchParams();
	const navigate = useNavigate();
	const location = useLocation();
	const canGoBack =
		(location.state as { pushed?: boolean } | null)?.pushed === true;

	const providerParam = searchParams.get("provider");
	const configIdParam = searchParams.get("configId");
	const newConfigParam = searchParams.get("newConfig");

	// Derive the current view from URL search params so that
	// browser back/forward navigation works as expected.
	const view: ProviderView =
		providerParam && providerStates.some((ps) => ps.provider === providerParam)
			? { mode: "detail", provider: providerParam }
			: { mode: "list" };

	// Clear provider search params and return to the list.
	const clearProviderView = () => {
		setSearchParams((prev) => clearProviderParams(new URLSearchParams(prev)));
	};

	// Detail view.
	const detailProvider =
		view.mode === "detail"
			? providerStates.find((ps) => ps.provider === view.provider)
			: undefined;
	const selectedConfig =
		!detailProvider || newConfigParam
			? undefined
			: (detailProvider.providerConfigs.find(
					({ id }) => id === configIdParam,
				) ??
				detailProvider.providerConfigs.find(
					({ id }) => id !== nilProviderConfigID,
				) ??
				detailProvider.providerConfigs[0]);
	const providerFormConfig =
		selectedConfig?.id === nilProviderConfigID ? undefined : selectedConfig;
	const providerFormProps =
		newConfigParam !== null || providerFormConfig !== undefined
			? { providerConfig: providerFormConfig }
			: {};

	if (view.mode === "detail" && detailProvider) {
		const providerFormKey = [
			detailProvider.provider,
			providerFormConfig?.id ?? "new",
			providerFormConfig?.display_name ?? "",
			providerFormConfig?.base_url ?? detailProvider.baseURL,
			providerFormConfig?.central_api_key_enabled ?? true,
			providerFormConfig?.allow_user_api_key ?? false,
			providerFormConfig?.allow_central_api_key_fallback ?? false,
			providerFormConfig?.has_api_key ?? detailProvider.hasManagedAPIKey,
			providerFormConfig?.updated_at ?? "",
		].join("|");

		return (
			<>
				{detailProvider.providerConfigs.length > 1 && (
					<div className="mb-4 grid gap-1.5">
						<Label className="text-[13px] font-medium text-content-primary">
							Configuration
						</Label>
						<Select
							value={selectedConfig?.id ?? ""}
							onValueChange={(id) => {
								setSearchParams((prev) => {
									const next = new URLSearchParams(prev);
									next.set("configId", id);
									next.delete("newConfig");
									return next;
								});
							}}
						>
							<SelectTrigger className="h-10 max-w-[320px] text-[13px]">
								<SelectValue placeholder="Select configuration" />
							</SelectTrigger>
							<SelectContent>
								{detailProvider.providerConfigs.map((providerConfig) => (
									<SelectItem key={providerConfig.id} value={providerConfig.id}>
										{formatProviderConfigLabel(providerConfig)}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
				)}
				{detailProvider.providerConfigs.length > 0 && (
					<Button
						variant="outline"
						size="sm"
						className="mb-4 gap-1.5"
						onClick={() => {
							setSearchParams(
								(prev) => {
									const next = new URLSearchParams(prev);
									next.delete("configId");
									next.set("newConfig", "1");
									return next;
								},
								{ state: { pushed: true } },
							);
						}}
					>
						<PlusIcon className="h-4 w-4" />
						New config
					</Button>
				)}
				<ProviderForm
					key={providerFormKey}
					providerState={detailProvider}
					{...providerFormProps}
					providerConfigsUnavailable={providerConfigsUnavailable}
					isProviderMutationPending={isProviderMutationPending}
					onCreateProvider={onCreateProvider}
					onUpdateProvider={onUpdateProvider}
					onDeleteProvider={async (id) => {
						await onDeleteProvider(id);
						if (canGoBack) {
							navigate(-1);
						} else {
							setSearchParams(
								(prev) => clearProviderParams(new URLSearchParams(prev)),
								{ replace: true },
							);
						}
					}}
					onBack={clearProviderView}
				/>
			</>
		);
	}

	// List view.
	if (providerStates.length === 0) {
		return (
			<div className="rounded-lg border border-dashed border-border bg-surface-primary p-6 text-center text-[13px] text-content-secondary">
				No provider types were returned by the backend.
			</div>
		);
	}

	return (
		<>
			{sectionLabel && (
				<SectionHeader
					label={sectionLabel}
					description={
						sectionDescription ?? "Configure AI providers to use with Agents."
					}
					badge={sectionBadge}
				/>
			)}
			<div>
				{providerStates.map((providerState, i) => (
					<button
						type="button"
						key={providerState.provider}
						aria-label={providerState.label}
						onClick={() => {
							onSelectedProviderChange(providerState.provider);
							setSearchParams(
								{ provider: providerState.provider },
								{ state: { pushed: true } },
							);
						}}
						className={cn(
							"flex w-full cursor-pointer items-center gap-3.5 border-0 bg-transparent p-0 px-3 py-3 text-left transition-colors hover:bg-surface-secondary/30",
							i > 0 && "border-0 border-t border-solid border-border/50",
						)}
					>
						<ProviderIcon
							provider={providerState.provider}
							className="h-8 w-8 shrink-0"
						/>
						<div className="min-w-0 flex-1 space-y-1">
							<div className="flex flex-wrap items-center gap-2">
								<span className="min-w-0 truncate text-[15px] font-medium text-content-primary text-left">
									{providerState.label}
								</span>
								{providerState.providerConfig?.allow_user_api_key && (
									<Badge size="xs" className="text-content-secondary">
										User keys enabled
									</Badge>
								)}
							</div>
						</div>
						{providerState.hasEffectiveAPIKey ? (
							<CheckCircleIcon className="h-4 w-4 shrink-0 text-content-success" />
						) : (
							<CircleIcon className="h-4 w-4 shrink-0 text-content-secondary opacity-40" />
						)}
						<ChevronRightIcon className="h-5 w-5 shrink-0 text-content-secondary" />
					</button>
				))}
			</div>
		</>
	);
};
