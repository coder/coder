import { type FC, type ReactNode, useReducer, useState } from "react";

import { useQuery } from "react-query";
import { templateBuilderModules } from "#/api/queries/templateBuilder";
import type {
	TemplateBuilderBasesResponse,
	TemplateBuilderModulesResponse,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";

import { docs } from "#/utils/docs";
import { BaseInfraSelectStep } from "./BaseInfraSelectStep";
import {
	BaseTemplateParametersStep,
	baseParametersComplete,
} from "./BaseTemplateParametersStep";
import { BuildingTemplateLoader } from "./BuildingTemplateLoader";
import { ModuleSelectStep } from "./ModuleSelectStep";
import {
	ModuleSettingsStep,
	moduleSettingsComplete,
} from "./ModuleSettingsStep";
import { SelectionSummary } from "./SelectionSummary";
import {
	findNextVisibleIndex,
	findPrevVisibleIndex,
	nearestVisible,
	type StepId,
	WIZARD_STEPS,
} from "./steps";
import { TemplateAlternatives } from "./TemplateAlternatives";
import { TemplateCustomizationsStep } from "./TemplateCustomizationsStep";
import {
	initialWizardState,
	type TemplateBuilderWizardState,
	type WizardAction,
	wizardReducer,
} from "./wizardState";

interface TemplateBuilderPageViewProps {
	error: unknown;
	basesData: TemplateBuilderBasesResponse | undefined;
	onCreateTemplate: (state: TemplateBuilderWizardState) => void;
	createError: Error | null;
	isCreating: boolean;
}

export const TemplateBuilderPageView: FC<TemplateBuilderPageViewProps> = ({
	error,
	basesData,
	onCreateTemplate,
	createError,
	isCreating,
}) => {
	const [state, dispatch] = useReducer(wizardReducer, initialWizardState);
	const [stepIndex, setStepIndex] = useState(0);
	const modulesQuery = useQuery(templateBuilderModules(state.selectedBase?.id));

	const moduleVarMap = Object.fromEntries(
		state.modules.map((m) => [m.id, m.variables ?? {}]),
	);

	const currentIndex = nearestVisible(stepIndex, state);
	const currentStep = WIZARD_STEPS[currentIndex];

	const nextIndex = findNextVisibleIndex(currentIndex, state);
	const prevIndex = findPrevVisibleIndex(currentIndex, state);
	const isFirstStep = prevIndex === -1;
	const isLastStep = nextIndex === -1;

	const canContinue = computeCanContinue(
		currentStep.id,
		state,
		basesData,
		modulesQuery.data,
		moduleVarMap,
	);

	const handleBack = () => {
		window.scrollTo(0, 0);
		setStepIndex(prevIndex);
	};

	const handleNext = () => {
		if (isLastStep) {
			onCreateTemplate(state);
			return;
		}
		window.scrollTo(0, 0);
		setStepIndex(nextIndex);
	};

	const handleDeselectModule = (moduleId: string) => {
		// If the only module gets deselected, go back to module selection
		if (state.modules.length === 1) {
			setStepIndex(WIZARD_STEPS.findIndex((s) => s.id === "module-select"));
		}
		dispatch({
			type: "SET_MODULES",
			modules: state.modules.filter((m) => m.id !== moduleId),
			meta: state.selectedModules.filter((m) => m.id !== moduleId),
		});
	};

	if (isCreating) {
		return <BuildingTemplateLoader />;
	}

	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>Create new template</PageHeaderTitle>
				<PageHeaderSubtitle>
					A Terraform blueprint for reproducible workspaces.
					<Link
						href={docs("/admin/templates")}
						target="_blank"
						className="ml-1 font-normal"
					>
						View docs
					</Link>
				</PageHeaderSubtitle>
			</PageHeader>

			{error != null && <ErrorAlert error={error} />}

			<div className="flex gap-8">
				{/* Main content area */}
				<div className="flex-1 min-w-0">
					<div className="p-6 border border-solid rounded-lg overflow-x-auto">
						{renderStepContent(
							currentStep.id,
							state,
							dispatch,
							moduleVarMap,
							createError,
						)}
					</div>

					{/* Navigation controls */}
					<div className="flex justify-end mt-6 gap-2">
						{isFirstStep ? (
							<div />
						) : (
							<Button variant="outline" onClick={handleBack}>
								Back
							</Button>
						)}
						<Button onClick={handleNext} disabled={!canContinue}>
							{isLastStep ? "Create Template" : "Continue"}
						</Button>
					</div>

					{currentStep.id === "base-infra" && <TemplateAlternatives />}
				</div>

				{/* Sidebar */}
				<div className="w-64 shrink-0 hidden md:block sticky top-0 self-start">
					<SelectionSummary
						currentStep={currentStep.group}
						selectedTemplate={
							state.selectedBase
								? {
										name: state.selectedBase.name,
										iconUrl: state.selectedBase.iconUrl,
									}
								: undefined
						}
						selectedModules={
							state.selectedModules.length > 0
								? state.selectedModules
								: undefined
						}
						onDeselectModule={handleDeselectModule}
					/>
				</div>
			</div>
		</Margins>
	);
};

function renderStepContent(
	stepId: StepId,
	state: TemplateBuilderWizardState,
	dispatch: (action: WizardAction) => void,
	moduleVarMap: Record<string, Record<string, string>>,
	createError: Error | null,
): ReactNode {
	switch (stepId) {
		case "base-infra":
			return (
				<BaseInfraSelectStep
					selectedBaseId={state.selectedBase?.id ?? null}
					onSelectBase={(base) => dispatch({ type: "SET_BASE", base })}
				/>
			);
		case "base-parameters":
			if (!state.selectedBase) return null;
			return (
				<BaseTemplateParametersStep
					baseId={state.selectedBase.id}
					values={state.baseVariableValues}
					onChangeValues={(values) =>
						dispatch({ type: "SET_BASE_VARIABLES", values })
					}
				/>
			);
		case "module-select":
			if (!state.selectedBase) return null;
			return (
				<ModuleSelectStep
					baseId={state.selectedBase.id}
					selectedModuleIds={state.modules.map((m) => m.id)}
					onChangeModules={(modules, meta) =>
						dispatch({ type: "SET_MODULES", modules, meta })
					}
				/>
			);
		case "module-settings":
			if (!state.selectedBase) return null;
			return (
				<ModuleSettingsStep
					baseId={state.selectedBase.id}
					selectedModuleIds={state.modules.map((m) => m.id)}
					moduleVariables={moduleVarMap}
					onChangeModuleVariables={(moduleId, variables) =>
						dispatch({
							type: "SET_MODULE_VARIABLES",
							moduleId,
							variables,
						})
					}
				/>
			);
		case "customizations":
			return (
				<>
					{createError != null && <ErrorAlert error={createError} />}
					<TemplateCustomizationsStep
						state={state}
						onChangeField={(field, value) =>
							dispatch({
								type: "SET_CUSTOMIZATION",
								field,
								value,
							})
						}
					/>
				</>
			);
		default:
			return null;
	}
}

function computeCanContinue(
	stepId: StepId,
	state: TemplateBuilderWizardState,
	basesData: TemplateBuilderBasesResponse | undefined,
	modulesData: TemplateBuilderModulesResponse | undefined,
	moduleVarMap: Record<string, Record<string, string>>,
): boolean {
	switch (stepId) {
		case "base-infra":
			return state.selectedBase !== null;
		case "base-parameters":
			return baseParametersComplete(
				basesData,
				state.selectedBase?.id ?? null,
				state.baseVariableValues,
			);
		case "module-settings":
			return moduleSettingsComplete(
				modulesData,
				state.modules.map((m) => m.id),
				moduleVarMap,
			);
		case "customizations":
			return state.name.trim() !== "";
		default:
			return true;
	}
}
