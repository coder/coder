import type { FC } from "react";
import { useQuery } from "react-query";
import { templateBuilderBases } from "#/api/queries/templateBuilder";
import type {
	TemplateBuilderBasesResponse,
	TemplateBuilderModuleVariable,
} from "#/api/typesGenerated";
import { MemoizedMarkdown } from "#/components/Markdown/Markdown";
import {
	TemplateBuilderSubtitle,
	TemplateBuilderTitle,
} from "#/pages/TemplateBuilder/TemplateBuilderHeader";
import { cn } from "#/utils/cn";
import type { ConfigurationFieldDefinition } from "./ConfigurationField";
import { TemplateConfiguration } from "./TemplateConfiguration";

interface BaseTemplateParametersStepProps {
	baseId: string;
	values: Record<string, string>;
	onChangeValues: (values: Record<string, string>) => void;
}

function detailsUrl(baseId: string): string {
	return `https://registry.coder.com/templates/${baseId}`;
}

/**
 * Maps a TemplateBuilderModuleVariable to a ConfigurationFieldDefinition,
 * using the controlled values from wizard state.
 */
function variableToField(
	variable: TemplateBuilderModuleVariable,
	value: string,
	onChange: (name: string, value: string) => void,
): ConfigurationFieldDefinition {
	const id = `base-var-${variable.name}`;

	if (variable.type === "bool") {
		return {
			type: "switch",
			id,
			label: variable.name,
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
		label: variable.name,
		description: variable.description || undefined,
		required: variable.required,
		placeholder: variable.required ? "Required" : "Optional",
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

/**
 * Returns true when all required, non-sensitive base variables have
 * a non-empty value. Returns true when no variables need filling.
 */
export function baseParametersComplete(
	bases: TemplateBuilderBasesResponse | undefined,
	baseId: string | null,
	values: Record<string, string>,
): boolean {
	if (!bases || !baseId) {
		return true;
	}
	const base = bases.bases.find((b) => b.id === baseId);
	if (!base) {
		return true;
	}
	const required = base.variables.filter((v) => v.required && !v.sensitive);
	return required.every((v) => {
		const val = values[v.name];
		return val !== undefined && val !== "";
	});
}

export const BaseTemplateParametersStep: FC<
	BaseTemplateParametersStepProps
> = ({ baseId, values, onChangeValues }) => {
	const { data } = useQuery(templateBuilderBases());
	const base = data?.bases.find((b) => b.id === baseId);
	const variables = base?.variables.filter((v) => !v.sensitive) ?? [];
	const prerequisites = base?.prerequisites ?? "";

	const handleChange = (name: string, value: string) => {
		onChangeValues({ ...values, [name]: value });
	};

	const fields: ConfigurationFieldDefinition[] = variables.map((v) =>
		variableToField(v, values[v.name] ?? "", handleChange),
	);

	return (
		<>
			<TemplateBuilderTitle>Configure base template</TemplateBuilderTitle>
			<TemplateBuilderSubtitle>
				Your base template requires customizations.
			</TemplateBuilderSubtitle>

			<TemplateConfiguration
				name={base?.name ?? "Base Template"}
				description={base?.description ?? ""}
				iconUrl={base?.icon}
				detailsUrl={detailsUrl(baseId)}
				fields={fields}
			>
				{prerequisites && (
					<div className="mt-6">
						<MemoizedMarkdown
							className={cn(
								"text-xs font-normal",
								"[&_h2]:mt-4 [&_h2]:text-sm [&_h2]:font-semibold",
								"[&_h3]:mt-4 [&_h3]:text-xs [&_h2]:font-semibold",
								"[&_p]:text-content-secondary",
								"[&_a]:text-xs [&_a]:font-normal",
								"[&_code]:text-xs",
							)}
						>
							{prerequisites}
						</MemoizedMarkdown>
					</div>
				)}
			</TemplateConfiguration>
		</>
	);
};
