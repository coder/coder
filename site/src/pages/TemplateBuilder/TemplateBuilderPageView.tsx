import {
	type FC,
	type ReactNode,
	useCallback,
	useEffect,
	useReducer,
	useRef,
} from "react";

import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
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
	furthestAllowedIndex,
	nearestVisible,
	type StepId,
	WIZARD_STEPS,
} from "./steps";
import { TemplateAlternatives } from "./TemplateAlternatives";
import { TemplateCustomizationsStep } from "./TemplateCustomizationsStep";
import {
	initWizardState,
	type SelectedBaseMeta,
	type TemplateBuilderWizardState,
	type WizardAction,
	wizardReducer,
} from "./wizardState";

interface TemplateBuilderPageViewProps {
	error: unknown;
	basesData: TemplateBuilderBasesResponse | undefined;
	preselectedBase?: SelectedBaseMeta;
	onCreateTemplate: (state: TemplateBuilderWizardState) => void;
	createError: Error | null;
	isCreating: boolean;
	onClearCreateError?: () => void;
	sessionId: string;
}

export const TemplateBuilderPageView: FC<TemplateBuilderPageViewProps> = ({
	error,
	basesData,
	preselectedBase,
	onCreateTemplate,
	createError,
	isCreating,
	onClearCreateError,
	sessionId,
}) => {
	const [state, dispatch] = useReducer(
		wizardReducer,
		{ sessionId, preselectedBase },
		initWizardState,
	);
	const [searchParams, setSearchParams] = useSearchParams();
	const modulesQuery = useQuery(templateBuilderModules(state.selectedBase?.id));

	const moduleVarMap = Object.fromEntries(
		state.modules.map((m) => [m.id, m.variables ?? {}]),
	);

	// The ?step= search param drives the current step so that browser
	// back/forward moves between steps. The requested step is clamped to
	// what the wizard state allows and snapped to the nearest visible
	// step, keeping the URL and state in sync even when the URL points at
	// a step the user cannot be on.
	const stepParam = searchParams.get("step");
	const requestedIndex = WIZARD_STEPS.findIndex((s) => s.id === stepParam);
	const defaultIndex = preselectedBase
		? Math.max(findNextVisibleIndex(0, state), 0)
		: 0;
	const clampedIndex = Math.min(
		requestedIndex >= 0 ? requestedIndex : defaultIndex,
		furthestAllowedIndex(state),
	);
	const currentIndex = nearestVisible(clampedIndex, state);
	const currentStep = WIZARD_STEPS[currentIndex];

	// Rewrite the URL whenever it disagrees with the resolved step.
	useEffect(() => {
		if (searchParams.get("step") === currentStep.id) {
			return;
		}
		const next = new URLSearchParams(searchParams);
		next.set("step", currentStep.id);
		setSearchParams(next, { replace: true });
	}, [currentStep.id, searchParams, setSearchParams]);

	// Reset scroll whenever the active step changes, including on browser
	// back/forward (popstate) where button click handlers would not fire.
	// biome-ignore lint/correctness/useExhaustiveDependencies: scroll must reset when step changes
	useEffect(() => {
		window.scrollTo(0, 0);
	}, [currentStep.id]);

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

	// Pushes a history entry so browser back/forward walks the steps.
	const navigateToStep = useCallback(
		(index: number) => {
			const next = new URLSearchParams(searchParams);
			next.set("step", WIZARD_STEPS[index].id);
			setSearchParams(next, { replace: false });
		},
		[searchParams, setSearchParams],
	);

	const handleBack = () => {
		if (currentStep.id === "customizations") {
			dispatch({ type: "RESET_CUSTOMIZATIONS" });
			onClearCreateError?.();
		}
		navigateToStep(prevIndex);
	};

	const handleNext = () => {
		if (isLastStep) {
			onCreateTemplate(state);
			return;
		}
		navigateToStep(nextIndex);
	};

	const handleProvisionerStatusChange = useCallback(
		(value: boolean | undefined) => {
			dispatch({ type: "SET_HAS_PROVISIONERS", value });
		},
		[],
	);

	const handleDeselectModule = (moduleId: string) => {
		// If the only module gets deselected, go back to module selection
		if (state.modules.length === 1) {
			navigateToStep(WIZARD_STEPS.findIndex((s) => s.id === "module-select"));
		}
		dispatch({
			type: "SET_MODULES",
			modules: state.modules.filter((m) => m.id !== moduleId),
			meta: state.selectedModules.filter((m) => m.id !== moduleId),
		});
	};

	// Maps module id -> its config section node, populated by
	// ModuleSettingsStep via callback refs. Used to scroll a module into
	// view without relying on DOM ids.
	const moduleRefs = useRef(new Map<string, HTMLDivElement>());

	const registerModuleRef = useCallback(
		(moduleId: string, node: HTMLDivElement | null) => {
			if (node) {
				moduleRefs.current.set(moduleId, node);
			} else {
				moduleRefs.current.delete(moduleId);
			}
		},
		[],
	);

	// Holds the module a sidebar click wants to scroll to, so the scroll can
	// happen after the module-settings step has rendered.
	const pendingModuleScrollRef = useRef<string | null>(null);

	const scrollModuleIntoView = (moduleId: string) => {
		moduleRefs.current.get(moduleId)?.scrollIntoView({ behavior: "smooth" });
	};

	// Sidebar module rows call this to jump to a module's configuration.
	const navigateToModule = (moduleId: string) => {
		const settingsIndex = WIZARD_STEPS.findIndex(
			(s) => s.id === "module-settings",
		);
		const settingsVisible =
			settingsIndex >= 0 && !WIZARD_STEPS[settingsIndex].shouldSkip(state);

		// If module-settings is skipped (no configurable vars) there is no
		// card to scroll to, so the click is a no-op.
		if (!settingsVisible) {
			return;
		}

		if (currentStep.id === "module-settings") {
			scrollModuleIntoView(moduleId);
			return;
		}
		// Remember the target and scroll once the step has rendered.
		pendingModuleScrollRef.current = moduleId;
		navigateToStep(settingsIndex);
	};

	// Runs after the scroll-reset effect above (declared earlier, so it fires
	// first). Scrolls the requested module into view once module-settings
	// has rendered.
	// biome-ignore lint/correctness/useExhaustiveDependencies: run on step change
	useEffect(() => {
		if (currentStep.id !== "module-settings") {
			return;
		}
		const moduleId = pendingModuleScrollRef.current;
		if (!moduleId) {
			return;
		}
		pendingModuleScrollRef.current = null;
		requestAnimationFrame(() => scrollModuleIntoView(moduleId));
	}, [currentStep.id]);

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
							handleProvisionerStatusChange,
							handleDeselectModule,
							registerModuleRef,
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

				{/* Sidebar (top position is 72px so that it can sit below nav) */}
				<div className="w-64 shrink-0 hidden md:block sticky top-[72px] self-start">
					<SelectionSummary
						currentStep={currentStep.group}
						onNavigateModule={navigateToModule}
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
	onProvisionerStatusChange: (value: boolean | undefined) => void,
	onRemoveModule: (moduleId: string) => void,
	registerModuleRef: (moduleId: string, node: HTMLDivElement | null) => void,
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
					onRemoveModule={onRemoveModule}
					registerModuleRef={registerModuleRef}
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
						onProvisionerStatusChange={onProvisionerStatusChange}
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
			return state.name.trim() !== "" && state.hasProvisioners !== false;
		default:
			return true;
	}
}
