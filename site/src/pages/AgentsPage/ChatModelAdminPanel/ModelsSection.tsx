import type * as TypesGen from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { PencilIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { type FC, useState } from "react";
import { cn } from "utils/cn";
import { formatProviderLabel } from "../modelOptions";
import type { ProviderState } from "./ChatModelAdminPanel";
import { ModelForm } from "./ModelForm";
import { ProviderIcon } from "./ProviderIcon";

type ModelView =
	| { mode: "list" }
	| { mode: "add" }
	| { mode: "edit"; model: TypesGen.ChatModelConfig };

type ModelsSectionProps = {
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

		// When editing, select the model's provider so the form shows
		// the correct provider-specific fields.
		const effectiveProvider = editingModel
			? editingModel.provider
			: selectedProvider;
		const effectiveProviderState = editingModel
			? (providerStates.find((ps) => ps.provider === editingModel.provider) ??
				null)
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
			/>
		);
	}

	// ── List view ──────────────────────────────────────────────

	return (
		<>
			<div className="space-y-4">
				{/* Add model button */}
				<div className="flex items-center justify-end">
					<Button
						size="sm"
						className="gap-1.5"
						onClick={() => setView({ mode: "add" })}
					>
						<PlusIcon className="h-4 w-4" />
						Add model
					</Button>
				</div>

				{/* Model list */}
				{modelConfigs.length === 0 ? (
					<div className="flex flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-border bg-surface-secondary/20 px-6 py-12 text-center">
						<div className="flex h-10 w-10 items-center justify-center rounded-lg bg-surface-tertiary/50">
							<PlusIcon className="h-5 w-5 text-content-secondary" />
						</div>
						<div>
							<p className="m-0 text-[13px] font-medium text-content-primary">
								No models configured
							</p>
							<p className="m-0 mt-1 text-xs text-content-secondary">
								Add a model to get started with Agents.
							</p>
						</div>
						<Button
							size="sm"
							variant="outline"
							className="mt-1 gap-1.5"
							onClick={() => setView({ mode: "add" })}
						>
							<PlusIcon className="h-3.5 w-3.5" />
							Add your first model
						</Button>
					</div>
				) : (
					<div className="divide-y divide-border overflow-hidden rounded-xl border border-border">
						{modelConfigs.map((modelConfig) => (
							<div
								key={modelConfig.id}
								className="group flex items-center gap-4 bg-surface-primary px-5 py-3.5 transition-colors hover:bg-surface-secondary/30"
							>
								<ProviderIcon
									provider={modelConfig.provider}
									className="h-8 w-8 shrink-0"
									active={modelConfig.enabled !== false}
								/>

								<div className="min-w-0 flex-1">
									<div className="flex items-center gap-2">
										<span
											className={cn(
												"truncate text-[13px] font-semibold",
												modelConfig.enabled === false
													? "text-content-secondary"
													: "text-content-primary",
											)}
										>
											{modelConfig.display_name || modelConfig.model}
										</span>
										{modelConfig.is_default && (
											<Badge size="sm" variant="info">
												default
											</Badge>
										)}
										{modelConfig.enabled === false && (
											<Badge size="sm" variant="warning">
												disabled
											</Badge>
										)}
									</div>
									<div className="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-content-secondary">
										<span className="inline-flex items-center gap-1">
											{formatProviderLabel(modelConfig.provider)}
										</span>
										<span className="font-mono">{modelConfig.model}</span>
										<span>
											{modelConfig.context_limit.toLocaleString()} ctx
										</span>
										<span>{modelConfig.compression_threshold}% compress</span>
									</div>
								</div>

								<div className="flex shrink-0 items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
									<Button
										size="icon"
										variant="subtle"
										className="h-8 w-8 text-content-secondary hover:text-content-primary"
										onClick={() =>
											setView({
												mode: "edit",
												model: modelConfig,
											})
										}
									>
										<PencilIcon className="h-4 w-4" />
										<span className="sr-only">Edit model</span>
									</Button>
									<Button
										size="icon"
										variant="subtle"
										className="h-8 w-8 text-content-secondary hover:text-content-destructive"
										onClick={() => setModelToDelete(modelConfig)}
										disabled={isDeleting}
									>
										<Trash2Icon className="h-4 w-4" />
										<span className="sr-only">Delete model</span>
									</Button>
								</div>
							</div>
						))}
					</div>
				)}
			</div>

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
