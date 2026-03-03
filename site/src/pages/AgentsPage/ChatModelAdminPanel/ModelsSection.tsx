import type * as TypesGen from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	ChevronDownIcon,
	ChevronRightIcon,
	PlusIcon,
	StarIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { cn } from "utils/cn";
import { SectionHeader } from "../SectionHeader";
import type { ProviderState } from "./ChatModelAdminPanel";
import { ModelForm } from "./ModelForm";
import { ProviderIcon } from "./ProviderIcon";

type ModelView =
	| { mode: "list" }
	| { mode: "add"; provider: string }
	| { mode: "edit"; model: TypesGen.ChatModelConfig };

type ModelsSectionProps = {
	sectionLabel?: string;
	providerStates: readonly ProviderState[];
	selectedProvider: string | null;
	selectedProviderState: ProviderState | null;
	onSelectedProviderChange: (provider: string) => void;
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
};

export const ModelsSection: FC<ModelsSectionProps> = ({
	sectionLabel,
	providerStates,
	selectedProvider,
	selectedProviderState,
	onSelectedProviderChange,
	modelConfigs,
	modelConfigsUnavailable,
	isCreating,
	isUpdating,
	isDeleting,
	onCreateModel,
	onUpdateModel,
	onDeleteModel,
}) => {
	const [view, setView] = useState<ModelView>({ mode: "list" });
	const [modelToDelete, setModelToDelete] =
		useState<TypesGen.ChatModelConfig | null>(null);

	// When the form is open it takes over the full panel.
	if (view.mode === "add" || view.mode === "edit") {
		const editingModel = view.mode === "edit" ? view.model : undefined;

		const getEffectiveProvider = () => {
			if (editingModel) {
				return editingModel.provider;
			}
			if (view.mode === "add") {
				return view.provider;
			}
			return selectedProvider;
		};

		const effectiveProvider = getEffectiveProvider();
		const effectiveProviderState = effectiveProvider
			? (providerStates.find((ps) => ps.provider === effectiveProvider) ?? null)
			: selectedProviderState;

		return (
			<ModelForm
				key={editingModel?.id ?? effectiveProvider ?? "new"}
				editingModel={editingModel}
				providerStates={providerStates}
				selectedProvider={effectiveProvider}
				selectedProviderState={effectiveProviderState}
				onSelectedProviderChange={onSelectedProviderChange}
				modelConfigsUnavailable={modelConfigsUnavailable}
				isSaving={isCreating || isUpdating}
				onCreateModel={async (req) => {
					await onCreateModel(req);
					setView({ mode: "list" });
				}}
				onUpdateModel={async (id, req) => {
					await onUpdateModel(id, req);
					setView({ mode: "list" });
				}}
				onCancel={() => setView({ mode: "list" })}
				onDeleteModel={
					editingModel
						? () => {
								setModelToDelete(editingModel);
								setView({ mode: "list" });
							}
						: undefined
				}
			/>
		);
	}

	// ── List view ──────────────────────────────────────────────

	// Only show providers that have an API key configured.
	const addableProviders = providerStates.filter(
		(ps) => ps.providerConfig && ps.hasEffectiveAPIKey,
	);

	const addButton = addableProviders.length > 0 && (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button size="sm" className="gap-1.5" aria-label="Add model">
					{" "}
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
							onSelectedProviderChange(ps.provider);
							setView({ mode: "add", provider: ps.provider });
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
		if (modelConfig.is_default) return;
		void onUpdateModel(modelConfig.id, { is_default: true });
	};

	return (
		<>
			{sectionLabel && (
				<SectionHeader
					label={sectionLabel}
					description="Manage models available to Agents."
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
							Connect a provider first to add models.
						</p>
					)}
				</div>
			) : (
				<div className="divide-y divide-border/50">
					{modelConfigs.map((modelConfig) => (
						<div
							key={modelConfig.id}
							className="flex items-center gap-3.5 px-3 py-3"
						>
							{" "}
							{/* Star for default */}
							<Tooltip>
								<TooltipTrigger asChild>
									<button
										type="button"
										onClick={(e) => {
											e.stopPropagation();
											handleSetDefault(modelConfig);
										}}
										aria-disabled={isUpdating || modelConfig.is_default}
										className={cn(
											"flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-transparent border-0 p-0 transition-colors",
											modelConfig.is_default
												? "text-yellow-400"
												: "cursor-pointer text-content-secondary/30 hover:text-content-secondary",
										)}
									>
										<StarIcon
											className={cn(
												"h-4 w-4",
												modelConfig.is_default && "fill-current",
											)}
										/>
									</button>
								</TooltipTrigger>
								<TooltipContent side="right">
									{modelConfig.is_default
										? "Default model for new chats"
										: "Set as default for new chats"}
								</TooltipContent>
							</Tooltip>
							{/* Clickable row content */}
							<button
								type="button"
								onClick={() => setView({ mode: "edit", model: modelConfig })}
								className="flex min-w-0 flex-1 cursor-pointer items-center gap-3.5 bg-transparent border-0 p-0 text-left transition-colors hover:opacity-80"
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
										{modelConfig.display_name || modelConfig.model}
									</span>
								</div>
								{modelConfig.enabled === false && (
									<Badge size="xs" variant="warning">
										disabled
									</Badge>
								)}
								<ChevronRightIcon className="h-5 w-5 shrink-0 text-content-secondary" />
							</button>{" "}
						</div>
					))}
				</div>
			)}

			<DeleteDialog
				isOpen={modelToDelete !== null}
				onCancel={() => setModelToDelete(null)}
				onConfirm={() => {
					if (modelToDelete) {
						void onDeleteModel(modelToDelete.id).finally(() =>
							setModelToDelete(null),
						);
					}
				}}
				entity="model"
				name={modelToDelete?.display_name || modelToDelete?.model || ""}
				confirmLoading={isDeleting}
			/>
		</>
	);
};
