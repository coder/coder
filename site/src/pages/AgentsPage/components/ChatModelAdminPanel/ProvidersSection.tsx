import {
	CheckCircleIcon,
	ChevronRightIcon,
	CircleIcon,
	PlusIcon,
} from "lucide-react";
import type { FC } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import {
	type AIProviderType,
	AIProviderTypes,
	type CreateChatProviderConfigRequest,
	type UpdateChatModelConfigRequest,
	type UpdateChatProviderConfigRequest,
} from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { cn } from "#/utils/cn";
import { formatProviderLabel } from "../../utils/modelOptions";
import { SectionHeader } from "../SectionHeader";
import type {
	CreateProviderResult,
	ProviderState,
} from "./ChatModelAdminPanel";
import { getDefaultProviderBaseURL } from "./helpers";
import { ProviderForm } from "./ProviderForm";
import { ProviderIcon } from "./ProviderIcon";

type ProviderView =
	| { mode: "list" }
	| { mode: "detail"; provider: string }
	| { mode: "new"; providerType: AIProviderType };

const providerTypeOptions = AIProviderTypes.map((providerType) => ({
	providerType,
	label: formatProviderLabel(providerType),
})).sort((a, b) => a.label.localeCompare(b.label));

const getAIProviderType = (
	value: string | null,
): AIProviderType | undefined => {
	if (!value) {
		return undefined;
	}
	return AIProviderTypes.find((providerType) => providerType === value);
};

const newProviderState = (providerType: AIProviderType): ProviderState => ({
	key: `new:${providerType}`,
	provider: providerType,
	label: formatProviderLabel(providerType),
	providerConfig: undefined,
	modelConfigs: [],
	catalogModelCount: 0,
	hasManagedAPIKey: false,
	hasCatalogAPIKey: false,
	hasEffectiveAPIKey: false,
	allowUserAPIKey: true,
	isEnvPreset: false,
	baseURL: getDefaultProviderBaseURL(providerType),
});

interface ProvidersSectionProps {
	sectionLabel?: string;
	sectionDescription?: string;
	providerStates: readonly ProviderState[];
	providerConfigsUnavailable: boolean;
	isProviderMutationPending: boolean;
	onCreateProvider: (
		req: CreateChatProviderConfigRequest,
	) => Promise<CreateProviderResult>;
	onUpdateProvider: (
		providerConfigId: string,
		req: UpdateChatProviderConfigRequest,
	) => Promise<unknown>;
	onDeleteProvider: (providerConfigId: string) => Promise<void>;
	onDisableModel: (
		modelConfigId: string,
		req: UpdateChatModelConfigRequest,
	) => Promise<unknown>;
}

export const ProvidersSection: FC<ProvidersSectionProps> = ({
	sectionLabel,
	sectionDescription,
	providerStates,
	providerConfigsUnavailable,
	isProviderMutationPending,
	onCreateProvider,
	onUpdateProvider,
	onDeleteProvider,
	onDisableModel,
}) => {
	const [searchParams, setSearchParams] = useSearchParams();
	const navigate = useNavigate();
	const location = useLocation();
	const canGoBack =
		(location.state as { pushed?: boolean } | null)?.pushed === true;

	// Derive the current view from URL search params so that
	// browser back/forward navigation works as expected.
	const view: ProviderView = (() => {
		const providerParam = searchParams.get("provider");
		if (providerParam) {
			const exists = providerStates.some((ps) => ps.key === providerParam);
			return exists
				? { mode: "detail", provider: providerParam }
				: { mode: "list" };
		}

		const newProviderType = getAIProviderType(searchParams.get("newProvider"));
		if (newProviderType) {
			return { mode: "new", providerType: newProviderType };
		}

		return { mode: "list" };
	})();

	// Clear provider search param and return to the list.
	const clearProviderView = () => {
		setSearchParams((prev) => {
			const next = new URLSearchParams(prev);
			next.delete("provider");
			next.delete("newProvider");
			return next;
		});
	};

	const openNewProviderView = (providerType: AIProviderType) => {
		setSearchParams(
			(prev) => {
				const next = new URLSearchParams(prev);
				next.delete("provider");
				next.set("newProvider", providerType);
				return next;
			},
			{ state: { pushed: true } },
		);
	};
	// Detail view.
	const detailProvider =
		view.mode === "detail"
			? providerStates.find((ps) => ps.key === view.provider)
			: view.mode === "new"
				? newProviderState(view.providerType)
				: undefined;

	if ((view.mode === "detail" || view.mode === "new") && detailProvider) {
		const providerFormKey = [
			detailProvider.provider,
			detailProvider.providerConfig?.id ?? "new",
			detailProvider.providerConfig?.display_name ?? "",
			detailProvider.providerConfig?.base_url ?? detailProvider.baseURL,
			detailProvider.providerConfig?.central_api_key_enabled ?? true,
			detailProvider.providerConfig?.allow_user_api_key ?? false,
			detailProvider.providerConfig?.allow_central_api_key_fallback ?? false,
			detailProvider.providerConfig?.has_api_key ??
				detailProvider.hasManagedAPIKey,
			detailProvider.providerConfig?.updated_at ?? "",
		].join("|");

		return (
			<ProviderForm
				key={providerFormKey}
				providerState={detailProvider}
				providerConfigsUnavailable={providerConfigsUnavailable}
				isProviderMutationPending={isProviderMutationPending}
				onCreateProvider={async (req) => {
					const createdProvider = await onCreateProvider(req);
					if (createdProvider.id) {
						setSearchParams(
							(prev) => {
								const next = new URLSearchParams(prev);
								next.set("provider", createdProvider.id);
								next.delete("newProvider");
								return next;
							},
							{ replace: true, state: location.state },
						);
					}
					return createdProvider;
				}}
				onUpdateProvider={onUpdateProvider}
				onDeleteProvider={async (id) => {
					await onDeleteProvider(id);
					if (canGoBack) {
						navigate(-1);
					} else {
						setSearchParams(
							(prev) => {
								const next = new URLSearchParams(prev);
								next.delete("provider");
								next.delete("newProvider");
								return next;
							},
							{ replace: true },
						);
					}
				}}
				onBack={clearProviderView}
				onDisableModel={onDisableModel}
			/>
		);
	}

	// List view.
	const addProviderAction = (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					variant="outline"
					size="sm"
					disabled={providerConfigsUnavailable || isProviderMutationPending}
				>
					<PlusIcon className="size-4" />
					Add provider
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end" className="w-56">
				{providerTypeOptions.map(({ providerType, label }) => (
					<DropdownMenuItem
						key={providerType}
						className="gap-2"
						onSelect={() => openNewProviderView(providerType)}
					>
						<ProviderIcon provider={providerType} className="size-5" />
						<span>{label}</span>
					</DropdownMenuItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	);

	const header = sectionLabel ? (
		<SectionHeader
			label={sectionLabel}
			description={
				sectionDescription ?? "Configure AI providers to use with Agents."
			}
			action={addProviderAction}
		/>
	) : null;

	return (
		<>
			{header}
			{providerStates.length === 0 ? (
				<div className="rounded-lg border border-dashed border-border bg-surface-primary p-6 text-center text-[13px] text-content-secondary">
					No providers have been added yet.
				</div>
			) : (
				<div>
					{providerStates.map((providerState, i) => (
						<button
							type="button"
							key={providerState.key}
							aria-label={providerState.label}
							onClick={() => {
								setSearchParams(
									{ provider: providerState.key },
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
								className="size-8 shrink-0"
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
								<CheckCircleIcon className="size-4 shrink-0 text-content-success" />
							) : (
								<CircleIcon className="size-4 shrink-0 text-content-secondary opacity-40" />
							)}
							<ChevronRightIcon className="size-5 shrink-0 text-content-secondary" />
						</button>
					))}
				</div>
			)}
		</>
	);
};
