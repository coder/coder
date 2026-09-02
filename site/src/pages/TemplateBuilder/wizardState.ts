import type {
	TemplateBuilderBase,
	TemplateBuilderBaseAgent,
	TemplateBuilderComposeModule,
	TemplateBuilderComposeRequest,
	TemplateBuilderCreateTemplateRequest,
	TemplateBuilderModule,
} from "#/api/typesGenerated";

/**
 * UI-only view of a base coder_agent. displayName is guaranteed non-empty:
 * it falls back to name when the API omits display_name.
 */
export type SelectedBaseAgent = {
	name: string;
	displayName: string;
	default: boolean;
};

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
	agents: SelectedBaseAgent[];
};

/**
 * Maps an API TemplateBuilderBase to the UI-only SelectedBaseMeta.
 */
export function toSelectedBaseMeta(
	base: TemplateBuilderBase,
): SelectedBaseMeta {
	const agents = base.agents.map(toSelectedBaseAgent);
	return {
		id: base.id,
		name: base.name,
		description: base.description,
		iconUrl: base.icon,
		os: base.os,
		hasParameters:
			base.variables?.length > 0 && base.variables?.some((v) => !v.sensitive),
		hasPrerequisites: Boolean(base.prerequisites?.length),
		agents,
	};
}

function toSelectedBaseAgent(
	agent: TemplateBuilderBaseAgent,
): SelectedBaseAgent {
	return {
		name: agent.name,
		displayName: agent.display_name || agent.name,
		default: agent.default,
	};
}

/** Name of the default agent modules attach to when they do not name one. */
export function defaultAgentName(
	agents: SelectedBaseAgent[],
): string | undefined {
	return agents.find((a) => a.default)?.name;
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
	hasProvisioners: boolean | undefined;
	name: string;
	displayName: string;
	description: string;
	icon: string;
	selectedBase: SelectedBaseMeta | null;
	selectedModules: SelectedModuleMeta[];
	/** Epoch millis when the wizard was entered, used for telemetry duration. */
	enteredAt: number;
	/** Stable ID shared across wizard_entry and compose_completion events. */
	sessionId: string;
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
	enteredAt: 0,
	sessionId: "",
};

/** Arguments for building a fresh wizard state on mount. */
type WizardInit = {
	/** Optional base template to preselect (from the ?base= param). */
	preselectedBase?: SelectedBaseMeta;
	/** Stable session ID shared across telemetry events for this mount. */
	sessionId: string;
};

/**
 * Builds the initial wizard state with a fresh telemetry session,
 * optionally preselecting a base template.
 */
export function initWizardState(init: WizardInit): TemplateBuilderWizardState {
	const state: TemplateBuilderWizardState = {
		...initialWizardState,
		enteredAt: Date.now(),
		sessionId: init.sessionId,
	};
	if (!init.preselectedBase) {
		return state;
	}
	return {
		...state,
		baseTemplateId: init.preselectedBase.id,
		selectedBase: init.preselectedBase,
		...baseCustomizationDefaults(init.preselectedBase),
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
			type: "SET_MODULE_AGENT";
			moduleId: string;
			agentName: string;
	  }
	| {
			type: "SET_CUSTOMIZATION";
			field: "name" | "displayName" | "description" | "icon";
			value: string;
	  }
	| { type: "SET_HAS_PROVISIONERS"; value: boolean | undefined }
	| { type: "RESET_CUSTOMIZATIONS" }
	| { type: "RESET" };

/**
 * Merges an incoming SET_MODULES entry with its previously selected state,
 * preserving prior variable values and agent choice. agentDefault is set only
 * for multi-agent bases; when undefined, agent_name is left as-is.
 */
function mergeSelectedModule(
	incoming: TemplateBuilderComposeModule,
	existing: TemplateBuilderComposeModule | undefined,
	agentDefault: string | undefined,
): TemplateBuilderComposeModule {
	return {
		...incoming,
		variables: incoming.variables ?? existing?.variables,
		...(agentDefault !== undefined && {
			agent_name: incoming.agent_name ?? existing?.agent_name ?? agentDefault,
		}),
	};
}

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
			const existingById = new Map(state.modules.map((m) => [m.id, m]));
			const base = state.selectedBase;
			// Only multi-agent bases carry a per-module agent choice; agentDefault
			// stays undefined otherwise so agent_name is left unset and the backend
			// uses the sole agent.
			const agentDefault =
				base && base.agents.length > 1
					? defaultAgentName(base.agents)
					: undefined;
			return {
				...state,
				modules: action.modules.map((incoming) =>
					mergeSelectedModule(
						incoming,
						existingById.get(incoming.id),
						agentDefault,
					),
				),
				selectedModules: action.meta,
			};
		}
		case "SET_MODULE_AGENT": {
			return {
				...state,
				modules: state.modules.map((m) =>
					m.id === action.moduleId ? { ...m, agent_name: action.agentName } : m,
				),
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
			// Re-detect provisioners when the step is re-entered.
			return {
				...state,
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
 * Values owned by the Formik-backed customizations step. Kept separate from
 * the reducer state, which only seeds their initial (base-derived) defaults.
 */
export type CustomizationsFormValues = {
	organization_id: string;
	name: string;
	display_name: string;
	description: string;
	icon: string;
};

/**
 * Project wizard state and the submitted customization values into the API
 * request shape for the create-template endpoint.
 */
export const toCreateTemplateRequest = (
	state: TemplateBuilderWizardState,
	customizations: CustomizationsFormValues,
): TemplateBuilderCreateTemplateRequest => {
	return {
		...toComposeRequest(state),
		organization_id: customizations.organization_id,
		name: customizations.name,
		display_name: customizations.display_name || undefined,
		description: customizations.description || undefined,
		icon: customizations.icon || undefined,
	};
};
