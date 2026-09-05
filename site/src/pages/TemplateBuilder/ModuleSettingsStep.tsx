import type { FC } from "react";
import { useQuery } from "react-query";
import {
	templateBuilderBases,
	templateBuilderModules,
} from "#/api/queries/templateBuilder";
import type {
	TemplateBuilderBaseAgent,
	TemplateBuilderModule,
	TemplateBuilderModulesResponse,
	TemplateBuilderModuleVariable,
} from "#/api/typesGenerated";
import {
	TemplateBuilderSubtitle,
	TemplateBuilderTitle,
} from "#/pages/TemplateBuilder/TemplateBuilderHeader";
import {
	type ConfigurationFieldDefinition,
	ConfigurationFieldLabel,
} from "./ConfigurationField";
import { defaultPlaceholder } from "./defaultPlaceholder";
import { ModuleConfiguration } from "./ModuleConfiguration";
import {
	AGENT_NAME_VARIABLE,
	baseAgents,
	defaultAgentName,
} from "./wizardState";

interface ModuleSettingsStepProps {
	baseId: string;
	selectedModuleIds: string[];
	moduleVariables: Record<string, Record<string, string>>;
	onChangeModuleVariables: (
		moduleId: string,
		variables: Record<string, string>,
	) => void;
	onRemoveModule: (moduleId: string) => void;
	registerModuleRef: (moduleId: string, node: HTMLDivElement | null) => void;
}

function variableToField(
	moduleId: string,
	variable: TemplateBuilderModuleVariable,
	value: string,
	onChange: (name: string, value: string) => void,
	agents: readonly TemplateBuilderBaseAgent[],
): ConfigurationFieldDefinition {
	const id = `mod-${moduleId}-${variable.name}`;
	const label = <ConfigurationFieldLabel variable={variable} />;

	// The backend only returns agent_name for multi-agent bases, so its presence
	// is the signal to render an agent picker whose value drives both the
	// module's agent_name and its agent_id.
	if (variable.name === AGENT_NAME_VARIABLE && agents.length > 0) {
		const options = agents.map((a) => ({
			value: a.name,
			label: a.display_name,
		}));
		// Guard against a stored value that names an agent this base does not
		// declare (e.g. a default carried over from another base).
		const selected = options.some((o) => o.value === value)
			? value
			: defaultAgentName(agents);
		return {
			type: "radio",
			id,
			label: variable.name,
			description: variable.description || undefined,
			value: selected,
			onChange: (val) => onChange(variable.name, val),
			options,
		};
	}

	if (variable.type === "bool") {
		return {
			type: "switch",
			id,
			label,
			description: variable.description || undefined,
			required: variable.required,
			checked: value === "true",
			onCheckedChange: (checked) =>
				onChange(variable.name, checked ? "true" : "false"),
		};
	}

	return {
		type: "text",
		id,
		label,
		description: variable.description || undefined,
		required: variable.required,
		placeholder:
			defaultPlaceholder(variable.default) ??
			(variable.required ? "Required" : "Optional"),
		field: {
			name: variable.name,
			id,
			value,
			onChange: (e) => onChange(variable.name, e.target.value),
			onBlur: () => {},
			error: false,
		},
	};
}

function moduleDetailsUrl(moduleId: string): string {
	return `https://registry.coder.com/modules/${moduleId}`;
}

/**
 * Returns true when all required, non-sensitive variables across all
 * selected modules have non-empty values.
 */
export function moduleSettingsComplete(
	modulesData: TemplateBuilderModulesResponse | undefined,
	selectedModuleIds: string[],
	moduleVariables: Record<string, Record<string, string>>,
): boolean {
	if (!modulesData) {
		return true;
	}
	const modulesById = new Map(modulesData.modules.map((m) => [m.id, m]));
	for (const moduleId of selectedModuleIds) {
		const mod = modulesById.get(moduleId);
		if (!mod) continue;
		const vars = moduleVariables[moduleId] ?? {};
		const required = mod.variables.filter((v) => v.required && !v.sensitive);
		for (const v of required) {
			const val = vars[v.name];
			if (val === undefined || val === "") {
				return false;
			}
		}
	}
	return true;
}

export const ModuleSettingsStep: FC<ModuleSettingsStepProps> = ({
	baseId,
	selectedModuleIds,
	moduleVariables,
	onChangeModuleVariables,
	onRemoveModule,
	registerModuleRef,
}) => {
	const { data } = useQuery(templateBuilderModules(baseId));
	const modules = data?.modules ?? [];

	const { data: basesData } = useQuery(templateBuilderBases());
	const agents = baseAgents(basesData, baseId);

	const selectedModules = selectedModuleIds
		.map((id) => modules.find((m) => m.id === id))
		.filter((m): m is TemplateBuilderModule => m != null);

	const handleChange = (moduleId: string, varName: string, value: string) => {
		const current = moduleVariables[moduleId] ?? {};
		onChangeModuleVariables(moduleId, { ...current, [varName]: value });
	};

	return (
		<>
			<TemplateBuilderTitle>Configure modules</TemplateBuilderTitle>
			<TemplateBuilderSubtitle>
				Set values for module variables.
			</TemplateBuilderSubtitle>

			<div className="flex flex-col gap-6">
				{selectedModules.map((mod) => {
					const configurableVars = mod.variables.filter((v) => !v.sensitive);
					const sensitiveVars = mod.variables.filter((v) => v.sensitive);
					const vars = moduleVariables[mod.id] ?? {};

					const toField = (v: TemplateBuilderModuleVariable) =>
						variableToField(
							mod.id,
							v,
							vars[v.name] ?? defaultPlaceholder(v.default) ?? "",
							(name, val) => handleChange(mod.id, name, val),
							agents,
						);

					// Agent selection always shows alongside the required fields, not in
					// the collapsed additional-settings group, even though it is optional.
					const isRequiredField = (v: TemplateBuilderModuleVariable) =>
						v.required || (v.name === AGENT_NAME_VARIABLE && agents.length > 0);
					const requiredFields = configurableVars
						.filter(isRequiredField)
						.map(toField);
					const optionalFields = configurableVars
						.filter((v) => !isRequiredField(v))
						.map(toField);

					return (
						<div
							key={mod.id}
							ref={(node) => registerModuleRef(mod.id, node)}
							className="scroll-mt-24"
						>
							<ModuleConfiguration
								name={mod.display_name}
								description={mod.description}
								iconUrl={mod.icon}
								detailsUrl={moduleDetailsUrl(mod.id)}
								fields={requiredFields}
								optionalFields={optionalFields}
								sensitiveVariables={sensitiveVars}
								onRemove={() => onRemoveModule(mod.id)}
							/>
						</div>
					);
				})}
			</div>
		</>
	);
};
