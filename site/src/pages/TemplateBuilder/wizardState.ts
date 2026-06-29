import type {
	TemplateBuilderComposeModule,
	TemplateBuilderComposeRequest,
	TemplateBuilderCreateTemplateRequest,
	TemplateBuilderModule,
} from "#/api/typesGenerated";

/**
 * UI-only metadata for the selected base template.
 * Kept separate from the API request payload.
 */
export type SelectedBaseMeta = {
	id: string;
	name: string;
	iconUrl?: string;
	os?: string;
	hasParameters: boolean;
	hasPrerequisites: boolean;
};

/**
 * UI-only metadata for a selected module.
 * Kept separate from the API request payload.
 */
export type SelectedModuleMeta = {
	id: string;
	name: string;
	iconUrl: string;
	hasConfigurableVars: boolean;
};

export type TemplateBuilderWizardState = {
	baseTemplateId: string | null;
	baseVariableValues: Record<string, string>;
	modules: TemplateBuilderComposeModule[];
	organizationId?: string;
	name: string;
	displayName: string;
	description: string;
	icon: string;
	selectedBase: SelectedBaseMeta | null;
	selectedModules: SelectedModuleMeta[];
};

export const initialWizardState: TemplateBuilderWizardState = {
	baseTemplateId: null,
	baseVariableValues: {},
	modules: [],
	name: "",
	displayName: "",
	description: "",
	icon: "",
	selectedBase: null,
	selectedModules: [],
};

export type WizardAction =
	| { type: "SET_BASE"; base: SelectedBaseMeta }
	| { type: "SET_BASE_VARIABLES"; values: Record<string, string> }
	| {
			type: "SET_MODULES";
			modules: TemplateBuilderComposeModule[];
			meta: SelectedModuleMeta[];
	  }
	| {
			type: "SET_MODULE_VARIABLES";
			moduleId: string;
			variables: Record<string, string>;
	  }
	| {
			type: "SET_CUSTOMIZATION";
			field: "organizationId" | "name" | "displayName" | "description" | "icon";
			value: string;
	  }
	| { type: "RESET" };

export function wizardReducer(
	state: TemplateBuilderWizardState,
	action: WizardAction,
): TemplateBuilderWizardState {
	switch (action.type) {
		case "SET_BASE": {
			const baseChanged = state.baseTemplateId !== action.base.id;
			return {
				...state,
				baseTemplateId: action.base.id,
				selectedBase: action.base,
				// Clear base variable values when base changes.
				baseVariableValues: baseChanged ? {} : state.baseVariableValues,
			};
		}
		case "SET_BASE_VARIABLES":
			return {
				...state,
				baseVariableValues: action.values,
			};
		case "SET_MODULES": {
			// Preserve existing variable values for modules that remain selected.
			const existingById = new Map(state.modules.map((m) => [m.id, m]));
			const merged = action.modules.map((incoming) => {
				const existing = existingById.get(incoming.id);
				if (existing?.variables && !incoming.variables) {
					return { ...incoming, variables: existing.variables };
				}
				return incoming;
			});
			return {
				...state,
				modules: merged,
				selectedModules: action.meta,
			};
		}
		case "SET_MODULE_VARIABLES": {
			return {
				...state,
				modules: state.modules.map((m) =>
					m.id === action.moduleId ? { ...m, variables: action.variables } : m,
				),
			};
		}
		case "SET_CUSTOMIZATION":
			return {
				...state,
				[action.field]: action.value,
			};
		case "RESET":
			return initialWizardState;
		default:
			return state;
	}
}

/**
 * Returns true when a module has at least one variable that should be
 * shown to the user for configuration (not sensitive, not computed).
 */
export const moduleHasConfigurableVars = (
	module: TemplateBuilderModule,
): boolean => {
	return module.variables.some((v) => !v.sensitive);
};

/**
 * Project wizard state into the API request shape for the compose endpoint.
 */
export const toComposeRequest = (
	state: TemplateBuilderWizardState,
): TemplateBuilderComposeRequest => {
	return {
		base_template_id: state.baseTemplateId ?? "",
		base_variable_values:
			Object.keys(state.baseVariableValues).length > 0
				? state.baseVariableValues
				: undefined,
		modules: state.modules,
	};
};

/**
 * Project wizard state into the API request shape for the
 * create-template endpoint.
 */
export const toCreateTemplateRequest = (
	state: TemplateBuilderWizardState,
): TemplateBuilderCreateTemplateRequest => {
	return {
		...toComposeRequest(state),
		organization_id: state.organizationId ?? "",
		name: state.name,
		display_name: state.displayName || undefined,
		description: state.description || undefined,
		icon: state.icon || undefined,
	};
};
