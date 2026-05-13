import {
	ChevronDownIcon,
	CopyIcon,
	PencilIcon,
	PlusIcon,
	StarIcon,
	TriangleAlertIcon,
} from "lucide-react";
import type { FC } from "react";
import { Link, useLocation, useSearchParams } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { SectionHeader } from "../SectionHeader";
import type { ProviderState } from "./ChatModelAdminPanel";
import { ModelForm } from "./ModelForm";
import { ProviderIcon } from "./ProviderIcon";
import { hasCustomPricing } from "./pricingFields";

type ModelView =
	| { mode: "list" }
	| { mode: "add"; provider: string }
	| { mode: "edit"; model: TypesGen.ChatModelConfig }
	| { mode: "duplicate"; sourceModel: TypesGen.ChatModelConfig };

const MODEL_VIEW_PARAMS = ["model", "newModel", "duplicate"] as const;
type ModelViewParam = (typeof MODEL_VIEW_PARAMS)[number];

const clearModelViewParams = (params: URLSearchParams) => {
	for (const param of MODEL_VIEW_PARAMS) {
		params.delete(param);
	}
};

const canManageProviderModels = (providerState: ProviderState | undefined) => {
	return Boolean(
		providerState?.providerConfig &&
			(providerState.hasEffectiveAPIKey ||
				providerState.providerConfig.allow_user_api_key),
	);
};

interface ModelsSectionProps {
	sectionLabel?: string;
	sectionDescription?: string;
	providerStates: readonly ProviderState[];
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	modelConfigsUnavailable: boolean;
	isCreating: boolean;
	isUpdating: boolean;
	isDeleting: boolean;
	onCreateModel: (
		req: TypesGen.CreateChatModelConfigRequest,
	) => Promise<unknown>;
	onUpdateModel: (
		modelConfigId: string,
		req: TypesGen.UpdateChatModelConfigRequest,
	) => Promise<unknown>;
	onDeleteModel: (modelConfigId: string) => Promise<void>;
}

export const ModelsSection: FC<ModelsSectionProps> = ({
	sectionLabel,
	sectionDescription,
	providerStates,
	modelConfigs,
	modelConfigsUnavailable,
	isCreating,
	isUpdating,
	isDeleting,
	onCreateModel,
	onUpdateModel,
	onDeleteModel,
}) => {
	const [searchParams, setSearchParams] = useSearchParams();
	const location = useLocation();

	// Derive the current view from URL search params so that
	// browser back/forward navigation works as expected.
	const view: ModelView = (() => {
		const editModelId = searchParams.get("model");
		if (editModelId) {
			const model = modelConfigs.find((m) => m.id === editModelId);
			return model ? { mode: "edit", model } : { mode: "list" };
		}
		const duplicateModelId = searchParams.get("duplicate");
		if (duplicateModelId) {
			const sourceModel = modelConfigs.find((m) => m.id === duplicateModelId);
			return sourceModel
				? { mode: "duplicate", sourceModel }
				: { mode: "list" };
		}
		const addProvider = searchParams.get("newModel");
		if (addProvider) {
			return { mode: "add", provider: addProvider };
		}
		return { mode: "list" };
	})();

	const setModelViewParam = (
		param: ModelViewParam,
		value: string,
		options?: { replace?: boolean },
	) => {
		const nextParams = new URLSearchParams(searchParams);
		clearModelViewParams(nextParams);
		nextParams.set(param, value);
		setSearchParams(nextParams, {
			replace: options?.replace,
			state: options?.replace ? location.state : { pushed: true },
		});
	};

	// Clear model-related search params and return to the list.
	const clearModelView = () => {
		setSearchParams((prev) => {
			const next = new URLSearchParams(prev);
			clearModelViewParams(next);
			return next;
		});
	};

	const exitModelView = () => {
		setSearchParams(
			(prev) => {
				const next = new URLSearchParams(prev);
				clearModelViewParams(next);
				return next;
			},
			{ replace: true },
		);
	};

	// When the form is open it takes over the full panel.
	if (
		view.mode === "add" ||
		view.mode === "edit" ||
		view.mode === "duplicate"
	) {
		const editingModel = view.mode === "edit" ? view.model : undefined;
		const duplicateSourceModel =
			view.mode === "duplicate" ? view.sourceModel : undefined;
		const effectiveProvider =
			view.mode === "edit"
				? view.model.provider
				: view.mode === "duplicate"
					? view.sourceModel.provider
					: view.provider;
		const effectiveProviderState =
			providerStates.find((ps) => ps.provider === effectiveProvider) ?? null;
		const formKey =
			view.mode === "edit"
				? `edit:${view.model.id}`
				: view.mode === "duplicate"
					? `duplicate:${view.sourceModel.id}`
					: `add:${view.provider}`;

		return (
			<ModelForm
				key={formKey}
				editingModel={editingModel}
				duplicateSourceModel={duplicateSourceModel}
				providerStates={providerStates}
				selectedProvider={effectiveProvider}
				selectedProviderState={effectiveProviderState}
				onSelectedProviderChange={(provider) => {
					if (view.mode === "add") {
						setModelViewParam("newModel", provider, { replace: true });
					}
				}}
				modelConfigsUnavailable={modelConfigsUnavailable}
				isSaving={isCreating || isUpdating}
				isDeleting={isDeleting}
				onCreateModel={async (req) => {
					await onCreateModel(req);
					exitModelView();
				}}
				onUpdateModel={async (id, req) => {
					await onUpdateModel(id, req);
					clearModelView();
				}}
				onCancel={clearModelView}
				onDeleteModel={
					editingModel
						? async (id) => {
								await onDeleteModel(id);
								exitModelView();
							}
						: undefined
				}
			/>
		);
	}

	// ── List view ──────────────────────────────────────────────

	// Only show providers that have a deployment key configured or allow
	// end users to bring their own key.
	const addableProviders = providerStates.filter(canManageProviderModels);

	const addButton = addableProviders.length > 0 && (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button size="sm" className="gap-1.5" aria-label="Add model">
					<PlusIcon className="h-4 w-4" />
					Add
					<ChevronDownIcon className="h-3.5 w-3.5 text-content-secondary" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				{addableProviders.map((ps) => (
					<DropdownMenuItem
						key={ps.provider}
						onClick={() => {
							setModelViewParam("newModel", ps.provider);
						}}
						className="gap-2"
					>
						<ProviderIcon provider={ps.provider} className="h-5 w-5" />
						{ps.label}
					</DropdownMenuItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	);

	const handleSetDefault = (modelConfig: TypesGen.ChatModelConfig) => {
		if (isUpdating || modelConfig.is_default || !modelConfig.enabled) return;
		void onUpdateModel(modelConfig.id, { is_default: true });
	};

	return (
		<>
			{sectionLabel && (
				<SectionHeader
					label={sectionLabel}
					description={
						sectionDescription ?? "Manage models available to Agents."
					}
					action={addButton || undefined}
				/>
			)}

			{modelConfigs.length === 0 ? (
				<div className="flex flex-col items-center justify-center gap-3 px-6 py-12 text-center">
					<p className="m-0 text-sm text-content-secondary">
						No models configured yet.
					</p>
					{addableProviders.length > 0 && addButton}
					{addableProviders.length === 0 && (
						<p className="m-0 text-xs text-content-secondary">
							Connect a{" "}
							<Link
								to="/agents/settings/providers"
								className="underline transition-colors hover:text-content-primary"
							>
								provider
							</Link>{" "}
							first to add models.
						</p>
					)}
				</div>
			) : (
				<div>
					{modelConfigs.map((modelConfig, i) => {
						const showPricingWarning = !hasCustomPricing(
							modelConfig.model_config,
						);
						const modelName = modelConfig.display_name || modelConfig.model;
						const starLabel = modelConfig.is_default
							? `Default model: ${modelName}`
							: `Set as default model: ${modelName}`;
						const starUnavailable =
							isUpdating || modelConfig.is_default || !modelConfig.enabled;
						const providerState = providerStates.find(
							(ps) => ps.provider === modelConfig.provider,
						);
						const duplicateUnavailable =
							!canManageProviderModels(providerState);

						return (
							<div
								key={modelConfig.id}
								className={cn(
									"flex items-center gap-3.5 px-3 py-3 transition-colors hover:bg-surface-secondary/30",
									i > 0 && "border-0 border-t border-solid border-border/50",
								)}
							>
								<button
									type="button"
									onClick={() => setModelViewParam("model", modelConfig.id)}
									aria-label={`Open model: ${modelName}`}
									className="flex min-w-0 flex-1 cursor-pointer items-center gap-3.5 border-0 bg-transparent p-0 text-left"
								>
									<ProviderIcon
										provider={modelConfig.provider}
										className="h-8 w-8 shrink-0"
									/>
									<div className="min-w-0 flex-1">
										<span
											className={cn(
												"block truncate text-[15px] font-medium",
												modelConfig.enabled === false
													? "text-content-secondary"
													: "text-content-primary",
											)}
										>
											{modelName}
										</span>
										{showPricingWarning && (
											<span className="mt-1 flex items-center gap-1 text-xs text-content-warning">
												<TriangleAlertIcon className="h-3.5 w-3.5 shrink-0" />
												Model pricing is not defined
											</span>
										)}
									</div>
									{modelConfig.enabled === false && (
										<Badge size="xs" variant="warning">
											disabled
										</Badge>
									)}
								</button>
								<div className="flex shrink-0 items-center gap-1">
									<Tooltip>
										<TooltipTrigger asChild>
											<Button
												size="icon"
												variant="subtle"
												onClick={(event) => {
													event.stopPropagation();
													handleSetDefault(modelConfig);
												}}
												aria-disabled={starUnavailable}
												aria-label={starLabel}
												className={cn(
													"hover:bg-surface-secondary",
													starUnavailable &&
														"cursor-not-allowed text-content-secondary/40 hover:bg-transparent hover:text-content-secondary/40",
													modelConfig.is_default && "text-content-primary",
												)}
											>
												<StarIcon
													className={cn(
														modelConfig.is_default && "fill-current",
													)}
												/>
											</Button>
										</TooltipTrigger>
										<TooltipContent side="top">
											{!modelConfig.enabled
												? "Cannot set a disabled model as default"
												: modelConfig.is_default
													? "Default for new conversations"
													: "Set as default for new conversations"}
										</TooltipContent>
									</Tooltip>
									<Tooltip>
										<TooltipTrigger asChild>
											<Button
												size="icon"
												variant="subtle"
												onClick={(event) => {
													event.stopPropagation();
													setModelViewParam("model", modelConfig.id);
												}}
												aria-label={`Edit model: ${modelName}`}
												className="hover:bg-surface-secondary"
											>
												<PencilIcon />
											</Button>
										</TooltipTrigger>
										<TooltipContent side="top">Edit model</TooltipContent>
									</Tooltip>
									<Tooltip>
										<TooltipTrigger asChild>
											<Button
												size="icon"
												variant="subtle"
												onClick={(event) => {
													event.stopPropagation();
													if (duplicateUnavailable) return;
													setModelViewParam("duplicate", modelConfig.id);
												}}
												aria-disabled={duplicateUnavailable}
												aria-label={`Duplicate model: ${modelName}`}
												className={cn(
													"hover:bg-surface-secondary",
													duplicateUnavailable &&
														"cursor-not-allowed text-content-secondary/40 hover:bg-transparent hover:text-content-secondary/40",
												)}
											>
												<CopyIcon />
											</Button>
										</TooltipTrigger>
										<TooltipContent side="top">
											{duplicateUnavailable
												? "Set an API key for this provider before duplicating models"
												: "Duplicate model"}
										</TooltipContent>
									</Tooltip>
								</div>
							</div>
						);
					})}
				</div>
			)}
		</>
	);
};
