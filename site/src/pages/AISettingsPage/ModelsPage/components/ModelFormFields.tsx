import type { FormikContextType } from "formik";
import { ChevronDownIcon, ChevronRightIcon, InfoIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { Link as RouterLink } from "react-router";
import { getVisibleProviderFields } from "#/api/chatModelOptions";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Input } from "#/components/Input/Input";
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "#/components/InputGroup/InputGroup";
import { Label } from "#/components/Label/Label";
import { Link } from "#/components/Link/Link";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { ProviderState } from "#/modules/aiModels/providerStates";
import {
	GeneralModelConfigFields,
	ModelConfigFields,
	PricingEstimateFields,
	ReasoningEffortConfigFields,
} from "#/pages/AgentsPage/components/ChatModelAdminPanel/ModelConfigFields";
import { ModelIdentifierField } from "#/pages/AgentsPage/components/ChatModelAdminPanel/ModelIdentifierField";
import type {
	ModelConfigFormBuildResult,
	ModelFormValues,
} from "#/pages/AgentsPage/components/ChatModelAdminPanel/modelConfigFormLogic";
import { cn } from "#/utils/cn";
import { docs } from "#/utils/docs";
import type { FormHelpers } from "#/utils/formUtils";
import { useOrganizationModelsPath } from "../organizationModels";
import { ModelFormProviderSelect } from "./ModelFormProviderSelect";
import { ModelOrganizationSelect } from "./ModelOrganizationSelect";

const CollapsibleSection: FC<{
	title: string;
	description: ReactNode;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	className?: string;
	contentClassName?: string;
	children: ReactNode;
}> = ({
	title,
	description,
	open,
	onOpenChange,
	className,
	contentClassName,
	children,
}) => {
	return (
		<Collapsible
			open={open}
			onOpenChange={onOpenChange}
			className={cn("p-4", className)}
		>
			<CollapsibleTrigger className="flex w-full cursor-pointer items-start gap-2 border-0 bg-transparent p-0 text-left transition-colors hover:text-content-primary">
				{open ? (
					<ChevronDownIcon className="mt-0.5 size-4 shrink-0 text-content-secondary" />
				) : (
					<ChevronRightIcon className="mt-0.5 size-4 shrink-0 text-content-secondary" />
				)}
				<div>
					<h3 className="m-0 text-sm font-medium text-content-primary">
						{title}
					</h3>
					<p className="m-0 text-xs text-content-secondary">{description}</p>
				</div>
			</CollapsibleTrigger>
			<CollapsibleContent>
				<div className={contentClassName}>{children}</div>
			</CollapsibleContent>
		</Collapsible>
	);
};

export const ModelFormFields: FC<{
	form: FormikContextType<ModelFormValues>;
	mode: "add" | "edit" | "duplicate";
	providerStates: readonly ProviderState[];
	selectedProviderState: ProviderState;
	selectedProviderKey: string;
	selectedProviderType: string;
	onProviderChange: (providerKey: string) => void;
	isDuplicating: boolean;
	isEditing: boolean;
	isSaving: boolean;
	isReadOnly: boolean;
	canSubmit: boolean;
	initialModel?: TypesGen.ChatModel;
	modelField: FormHelpers;
	contextLimitField: FormHelpers;
	compressionThresholdField: FormHelpers;
	displayNameField: FormHelpers;
	setDefaultDisabled: boolean;
	modelConfigFormBuildResult: ModelConfigFormBuildResult;
	showCostEstimate: boolean;
	setShowCostEstimate: (open: boolean) => void;
	showProviderConfig: boolean;
	setShowProviderConfig: (open: boolean) => void;
	showAdvanced: boolean;
	setShowAdvanced: (open: boolean) => void;
}> = ({
	form,
	mode,
	providerStates,
	selectedProviderState,
	selectedProviderKey,
	selectedProviderType,
	onProviderChange,
	isDuplicating,
	isEditing,
	isSaving,
	isReadOnly,
	canSubmit,
	initialModel,
	modelField,
	contextLimitField,
	compressionThresholdField,
	displayNameField,
	setDefaultDisabled,
	modelConfigFormBuildResult,
	showCostEstimate,
	setShowCostEstimate,
	showProviderConfig,
	setShowProviderConfig,
	showAdvanced,
	setShowAdvanced,
}) => {
	const modelsPath = useOrganizationModelsPath();
	const hasProviderConfigFields =
		getVisibleProviderFields(selectedProviderState.provider).length > 0;

	return (
		<div className="border border-solid p-6 rounded-lg">
			<form
				onSubmit={form.handleSubmit}
				spellCheck={false}
				autoComplete="off"
				className="flex flex-col gap-6"
			>
				<div className="grid items-start gap-4 sm:grid-cols-2">
					<ModelFormProviderSelect
						providerStates={providerStates}
						selectedProviderKey={selectedProviderKey}
						isEditing={mode === "edit"}
						onProviderChange={onProviderChange}
						disabled={
							isDuplicating || isReadOnly || providerStates.length === 0
						}
					/>
					<div className="flex flex-col gap-1">
						<ModelIdentifierField
							form={form}
							modelField={modelField}
							mode={mode}
							selectedProvider={selectedProviderType}
							disabled={isSaving || isReadOnly}
							controlClassName="shadow-none"
						/>
						<label
							htmlFor="isDefault"
							className="flex w-fit cursor-pointer items-center gap-2 font-normal text-sm leading-6 text-content-secondary"
						>
							<Checkbox
								id="isDefault"
								checked={form.values.isDefault}
								onCheckedChange={(checked) =>
									form.setFieldValue("isDefault", checked === true)
								}
								disabled={setDefaultDisabled || isReadOnly}
							/>
							Set as Coder Agents default model
						</label>
					</div>
					<div className="grid gap-1.5">
						<Label
							htmlFor={displayNameField.id}
							className="flex items-center gap-1 leading-6 text-content-primary"
						>
							Display name{" "}
							<span className="text-xs font-bold text-content-destructive">
								*
							</span>
						</Label>
						<p className="m-0 text-xs text-content-secondary">
							Friendly name. Defaults to identifier if blank.
						</p>
						<Input
							id={displayNameField.id}
							name={displayNameField.name}
							className="placeholder:text-content-disabled shadow-none"
							placeholder={initialModel?.model ?? "Model name"}
							value={displayNameField.value}
							onChange={displayNameField.onChange}
							onBlur={displayNameField.onBlur}
							disabled={isSaving || isReadOnly}
						/>
					</div>
					<div className="grid gap-1.5">
						<Label
							htmlFor={contextLimitField.id}
							className="flex items-center gap-1 leading-6 text-content-primary"
						>
							Context limit{" "}
							<span className="text-xs font-bold text-content-destructive">
								*
							</span>
						</Label>
						{contextLimitField.error ? (
							<p className="m-0 text-xs text-content-destructive">
								{contextLimitField.helperText}
							</p>
						) : (
							<p className="m-0 text-xs text-content-secondary">
								Max tokens in the context window.
							</p>
						)}
						<InputGroup
							className={cn(
								contextLimitField.error && "border-border-destructive",
							)}
						>
							<InputGroupInput
								id={contextLimitField.id}
								name={contextLimitField.name}
								className="min-w-0 placeholder:text-content-disabled"
								placeholder="200000"
								value={contextLimitField.value}
								onChange={contextLimitField.onChange}
								onBlur={contextLimitField.onBlur}
								disabled={isSaving || isReadOnly}
								aria-invalid={contextLimitField.error}
							/>
							<InputGroupAddon align="inline-end">
								<span className="text-xs text-content-disabled">Tokens</span>
							</InputGroupAddon>
						</InputGroup>
					</div>
					<ModelOrganizationSelect
						label="Organization"
						readOnly={mode === "edit"}
						requireCreatePermission={mode !== "edit"}
					/>
				</div>

				<div className="overflow-hidden rounded-lg border border-solid border-border">
					<CollapsibleSection
						title="Cost estimate"
						description={
							<>
								Estimated price per million tokens in USD. Prices are read-only.{" "}
								Model prices are managed by AI Gateway.{" "}
								<Link
									href={docs(
										"/ai-coder/ai-gateway/cost-controls#configure-model-prices",
									)}
									size="sm"
								>
									Learn how to configure model prices.
								</Link>
							</>
						}
						open={showCostEstimate}
						onOpenChange={setShowCostEstimate}
						contentClassName="grid grid-cols-2 gap-3 pt-3 pl-6 sm:grid-cols-4"
					>
						<PricingEstimateFields
							provider={selectedProviderType}
							model={form.values.model}
						/>
					</CollapsibleSection>

					{hasProviderConfigFields && (
						<CollapsibleSection
							title="Provider configuration"
							description="Tune provider-specific behavior like reasoning, tool calling, and web search."
							open={showProviderConfig}
							onOpenChange={setShowProviderConfig}
							className="border-0 border-t border-solid border-border"
							contentClassName="pt-3 pl-6"
						>
							<ModelConfigFields
								provider={selectedProviderState.provider}
								form={form}
								fieldErrors={modelConfigFormBuildResult.fieldErrors}
								disabled={isSaving || isReadOnly}
							>
								<ReasoningEffortConfigFields
									provider={selectedProviderState.provider}
									form={form}
									fieldErrors={modelConfigFormBuildResult.fieldErrors}
									disabled={isSaving || isReadOnly}
								/>
							</ModelConfigFields>
						</CollapsibleSection>
					)}

					<CollapsibleSection
						title="Advanced"
						description="Low-level parameters like temperature and penalties. Rarely need changing."
						open={showAdvanced}
						onOpenChange={setShowAdvanced}
						className="border-0 border-t border-solid border-border"
						contentClassName="grid grid-cols-2 gap-3 pt-3 pl-6 sm:grid-cols-3"
					>
						<GeneralModelConfigFields
							provider={selectedProviderState.provider}
							form={form}
							fieldErrors={modelConfigFormBuildResult.fieldErrors}
							disabled={isSaving || isReadOnly}
						/>
						<div className="flex min-w-0 flex-col gap-1.5">
							<Label
								htmlFor={compressionThresholdField.id}
								className="flex items-center gap-1 leading-6 text-content-primary"
							>
								Compression threshold
								<Tooltip>
									<TooltipTrigger asChild>
										<InfoIcon className="size-3 text-content-secondary" />
									</TooltipTrigger>
									<TooltipContent side="top" className="max-w-[240px]">
										Percentage at which context is compressed.
									</TooltipContent>
								</Tooltip>
							</Label>
							<InputGroup
								className={cn(
									compressionThresholdField.error &&
										"border-border-destructive",
								)}
							>
								<InputGroupInput
									id={compressionThresholdField.id}
									name={compressionThresholdField.name}
									className="placeholder:text-content-disabled"
									placeholder="70"
									value={compressionThresholdField.value}
									onChange={compressionThresholdField.onChange}
									onBlur={compressionThresholdField.onBlur}
									disabled={isSaving || isReadOnly}
									aria-invalid={compressionThresholdField.error}
								/>
								<InputGroupAddon align="inline-end">
									<span className="text-xs text-content-disabled">%</span>
								</InputGroupAddon>
							</InputGroup>
							{compressionThresholdField.error && (
								<p className="m-0 text-xs text-content-destructive">
									{compressionThresholdField.helperText}
								</p>
							)}
						</div>
					</CollapsibleSection>
				</div>

				<div className="flex items-center justify-end gap-3">
					<RouterLink to={modelsPath}>
						<Button variant="outline" type="button">
							Cancel
						</Button>
					</RouterLink>
					{!isReadOnly && (
						<Button type="submit" disabled={!canSubmit}>
							{isSaving && <Spinner loading />}
							{isEditing
								? "Update model"
								: isDuplicating
									? "Create duplicate"
									: "Add Model"}
						</Button>
					)}
				</div>
			</form>
		</div>
	);
};
