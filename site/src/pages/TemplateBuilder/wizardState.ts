import type {
	TemplateBuilderBase,
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
	description?: string;
	iconUrl?: string;
	os?: string;
	hasParameters: boolean;
	hasPrerequisites: boolean;
};

/**
 * Maps an API TemplateBuilderBase to the UI-only SelectedBaseMeta.
 */
export function toSelectedBaseMeta(
	base: TemplateBuilderBase,
): SelectedBaseMeta {
	return {
		id: base.id,
		name: base.name,
		description: base.description,
		iconUrl: base.icon,
		os: base.os,
		hasParameters:
			base.variables?.length > 0 && base.variables?.some((v) => !v.sensitive),
		hasPrerequisites: Boolean(base.prerequisites?.length),
	};
}

/**
 * Derives editable customization defaults from the selected base template.
 * Empty base values fall through to the fields' existing placeholders.
 */
export function baseCustomizationDefaults(base: SelectedBaseMeta): {
	name: string;
	displayName: string;
	description: string;
	icon: string;
} {
	return {
		name: base.id,
		displayName: base.name,
		description: base.description ?? "",
		icon: base.iconUrl ?? "",
	};
}

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
	hasProvisioners: boolean | undefined;
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
	hasProvisioners: undefined,
	name: "",
	displayName: "",
	description: "",
	icon: "",
	selectedBase: null,
	selectedModules: [],
};

/**
 * Builds the initial wizard state, optionally preselecting a base
 * template.
 */
export function initWizardState(
	preselectedBase?: SelectedBaseMeta,
): TemplateBuilderWizardState {
	if (!preselectedBase) {
		return initialWizardState;
	}
	return {
		...initialWizardState,
		baseTemplateId: preselectedBase.id,
		selectedBase: preselectedBase,
		...baseCustomizationDefaults(preselectedBase),
	};
}

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
	| { type: "SET_HAS_PROVISIONERS"; value: boolean | undefined }
	| { type: "RESET_CUSTOMIZATIONS" }
	| { type: "RESET" };

export function wizardReducer(
	state: TemplateBuilderWizardState,
	action: WizardAction,
): TemplateBuilderWizardState {
	switch (action.type) {
		case "SET_BASE": {
			const baseChanged = state.baseTemplateId !== action.base.id;
			if (!baseChanged) {
				return { ...state, selectedBase: action.base };
			}
			// Changing the base clears base variable values and re-seeds the
			// customization fields with defaults derived from the new base.
			return {
				...state,
				baseTemplateId: action.base.id,
				selectedBase: action.base,
				baseVariableValues: {},
				...baseCustomizationDefaults(action.base),
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
		case "SET_HAS_PROVISIONERS":
			return {
				...state,
				hasProvisioners: action.value,
			};
		case "RESET_CUSTOMIZATIONS":
			// Reset only organization and provisioner detection so re-entering the
			// step re-runs org auto-select cleanly. The base-derived fields are
			// left intact (they are re-seeded by SET_BASE when the base changes).
			return {
				...state,
				organizationId: undefined,
				hasProvisioners: undefined,
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
