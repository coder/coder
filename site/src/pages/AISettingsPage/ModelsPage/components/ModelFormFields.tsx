import type { FormikContextType } from "formik";
import { ChevronDownIcon, ChevronRightIcon, InfoIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { Link } from "react-router";
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
	PricingModelConfigFields,
} from "#/pages/AgentsPage/components/ChatModelAdminPanel/ModelConfigFields";
import { ModelIdentifierField } from "#/pages/AgentsPage/components/ChatModelAdminPanel/ModelIdentifierField";
import type {
	ModelConfigFormBuildResult,
	ModelFormValues,
} from "#/pages/AgentsPage/components/ChatModelAdminPanel/modelConfigFormLogic";
import { cn } from "#/utils/cn";
import type { FormHelpers } from "#/utils/formUtils";
import { ModelFormProviderSelect } from "./ModelFormProviderSelect";

const CollapsibleSection: FC<{
	title: string;
	description: string;
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
	canSubmit: boolean;
	initialModel?: TypesGen.ChatModelConfig;
	modelField: FormHelpers;
	contextLimitField: FormHelpers;
	compressionThresholdField: FormHelpers;
	displayNameField: FormHelpers;
	setDefaultDisabled: boolean;
	modelConfigFormBuildResult: ModelConfigFormBuildResult;
	showPricing: boolean;
	setShowPricing: (open: boolean) => void;
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
	canSubmit,
	initialModel,
	modelField,
	contextLimitField,
	compressionThresholdField,
	displayNameField,
	setDefaultDisabled,
	modelConfigFormBuildResult,
	showPricing,
	setShowPricing,
	showProviderConfig,
	setShowProviderConfig,
	showAdvanced,
	setShowAdvanced,
}) => {
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
						onProviderChange={onProviderChange}
						disabled={isDuplicating || providerStates.length === 0}
					/>
					<div className="flex flex-col gap-1">
						<ModelIdentifierField
							form={form}
							modelField={modelField}
							mode={mode}
							selectedProvider={selectedProviderType}
							disabled={isSaving}
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
								disabled={setDefaultDisabled}
							/>
							Set as default model
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
							disabled={isSaving}
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
								disabled={isSaving}
								aria-invalid={contextLimitField.error}
							/>
							<InputGroupAddon align="inline-end">
								<span className="text-xs text-content-disabled">Tokens</span>
							</InputGroupAddon>
						</InputGroup>
					</div>
				</div>

				<div className="overflow-hidden rounded-lg border border-solid border-border">
					<CollapsibleSection
						title="Cost tracking"
						description="Set per-token pricing so Coder can track costs and enforce spending limits."
						open={showPricing}
						onOpenChange={setShowPricing}
						contentClassName="grid grid-cols-2 gap-3 pt-3 pl-6 sm:grid-cols-4"
					>
						<PricingModelConfigFields
							provider={selectedProviderState.provider}
							form={form}
							fieldErrors={modelConfigFormBuildResult.fieldErrors}
							disabled={isSaving}
						/>
					</CollapsibleSection>

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
							disabled={isSaving}
						/>
					</CollapsibleSection>

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
							disabled={isSaving}
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
									disabled={isSaving}
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
					<Link to="/ai/settings/models">
						<Button variant="outline" type="button">
							Cancel
						</Button>
					</Link>
					<Button type="submit" disabled={!canSubmit}>
						{isSaving && <Spinner loading />}
						{isEditing
							? "Update model"
							: isDuplicating
								? "Create duplicate"
								: "Add Model"}
					</Button>
				</div>
			</form>
		</div>
	);
};
