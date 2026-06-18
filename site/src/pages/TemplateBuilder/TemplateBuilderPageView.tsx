import { type FC, useReducer, useState } from "react";
import { useQuery } from "react-query";
import {
	templateBuilderBases,
	templateBuilderModules,
} from "#/api/queries/templateBuilder";
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
	WIZARD_STEPS,
} from "./steps";
import { initialWizardState, wizardReducer } from "./wizardState";

interface TemplateBuilderPageViewProps {
	error: unknown;
}

export const TemplateBuilderPageView: FC<TemplateBuilderPageViewProps> = ({
	error,
}) => {
	const [state, dispatch] = useReducer(wizardReducer, initialWizardState);
	const [stepIndex, setStepIndex] = useState(0);
	const basesQuery = useQuery(templateBuilderBases());
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

	const canContinue =
		currentStep.id === "base-parameters"
			? baseParametersComplete(
					basesQuery.data,
					state.selectedBase?.id ?? null,
					state.baseVariableValues,
				)
			: currentStep.id === "module-settings"
				? moduleSettingsComplete(
						modulesQuery.data,
						state.modules.map((m) => m.id),
						moduleVarMap,
					)
				: true;

	const handleBack = () => {
		setStepIndex(prevIndex);
	};

	const handleNext = () => {
		if (isLastStep) {
			// Compose will be wired in a follow-up issue.
			return;
		}
		setStepIndex(nextIndex);
	};

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

	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>Create new template</PageHeaderTitle>
				<PageHeaderSubtitle>
					A Terraform blueprint for reproducible workspaces
					<Link href={docs("/admin/templates")} className="ml-1">
						View docs
					</Link>
				</PageHeaderSubtitle>
			</PageHeader>

			{error != null && <ErrorAlert error={error} />}

			<div className="flex gap-8">
				{/* Main content area */}
				<div className="flex-1 min-w-0">
					{currentStep.id === "base-infra" ? (
						<BaseInfraSelectStep
							selectedBaseId={state.selectedBase?.id ?? null}
							onSelectBase={(base) => dispatch({ type: "SET_BASE", base })}
						/>
					) : currentStep.id === "base-parameters" && state.selectedBase ? (
						<BaseTemplateParametersStep
							baseId={state.selectedBase.id}
							values={state.baseVariableValues}
							onChangeValues={(values) =>
								dispatch({ type: "SET_BASE_VARIABLES", values })
							}
						/>
					) : currentStep.id === "module-select" && state.selectedBase ? (
						<ModuleSelectStep
							baseId={state.selectedBase.id}
							selectedModuleIds={state.modules.map((m) => m.id)}
							onChangeModules={(modules, meta) =>
								dispatch({ type: "SET_MODULES", modules, meta })
							}
						/>
					) : currentStep.id === "module-settings" && state.selectedBase ? (
						<ModuleSettingsStep
							baseId={state.selectedBase.id}
							selectedModuleIds={state.modules.map((m) => m.id)}
							moduleVariables={moduleVarMap}
							onChangeModuleVariables={(moduleId, variables) =>
								dispatch({ type: "SET_MODULE_VARIABLES", moduleId, variables })
							}
						/>
					) : (
						<div className="rounded-lg border border-solid border-border bg-surface-primary p-6 min-h-[400px]">
							<p className="text-sm text-content-secondary">
								Step: {currentStep.id}
							</p>
						</div>
					)}

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
				</div>

				{/* Sidebar */}
				<div className="w-64 shrink-0 hidden md:block">
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
