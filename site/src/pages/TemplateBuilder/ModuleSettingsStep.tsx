import { InfoIcon } from "lucide-react";
import type { FC } from "react";
import { useQuery } from "react-query";
import { templateBuilderModules } from "#/api/queries/templateBuilder";
import type {
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

interface ModuleSettingsStepProps {
	baseId: string;
	selectedModuleIds: string[];
	moduleVariables: Record<string, Record<string, string>>;
	onChangeModuleVariables: (
		moduleId: string,
		variables: Record<string, string>,
	) => void;
}

function variableToField(
	moduleId: string,
	variable: TemplateBuilderModuleVariable,
	value: string,
	onChange: (name: string, value: string) => void,
): ConfigurationFieldDefinition {
	const id = `mod-${moduleId}-${variable.name}`;
	const label = <ConfigurationFieldLabel variable={variable} />;

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
}) => {
	const { data } = useQuery(templateBuilderModules(baseId));
	const modules = data?.modules ?? [];

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

			{/* 340px accounts for navbar, page header, card padding, and nav controls */}
			<div className="flex flex-col gap-6 max-h-[calc(100vh-340px)] overflow-y-auto">
				{selectedModules.map((mod) => {
					const configurableVars = mod.variables.filter((v) => !v.sensitive);
					const sensitiveVars = mod.variables.filter((v) => v.sensitive);
					const vars = moduleVariables[mod.id] ?? {};

					const toField = (v: TemplateBuilderModuleVariable) =>
						variableToField(mod.id, v, vars[v.name] ?? "", (name, val) =>
							handleChange(mod.id, name, val),
						);

					const requiredVars = configurableVars.filter((v) => v.required);
					const optionalVars = configurableVars.filter((v) => !v.required);

					const requiredFields = requiredVars.map(toField);
					const optionalFields = optionalVars.map(toField);

					return (
						<div key={mod.id}>
							<ModuleConfiguration
								name={mod.display_name}
								description={mod.description}
								iconUrl={mod.icon}
								detailsUrl={moduleDetailsUrl(mod.id)}
								fields={requiredFields}
								optionalFields={optionalFields}
							/>

							{sensitiveVars.length > 0 && (
								<div className="flex items-center gap-2 mt-2 p-3 rounded-md text-sm text-content-secondary">
									<InfoIcon className="size-icon-sm shrink-0 mt-0.5" />
									<p>
										{sensitiveVars.map((v) => (
											<code
												key={v.name}
												className="mr-1 px-1.5 py-1 bg-surface-secondary"
											>
												{v.name}
											</code>
										))}
										will be collected from developers at workspace creation.
									</p>
								</div>
							)}
						</div>
					);
				})}
			</div>
		</>
	);
};
