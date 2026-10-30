import { useFormik } from "formik";
import { XIcon } from "lucide-react";
import { useQuery } from "react-query";
import { richParameters } from "#/api/queries/templates";
import type {
	ChatProjectTemplateParameterRule,
	TemplateVersionParameter,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { TagInput } from "#/components/TagInput/TagInput";
import { OptionCheckbox } from "./OptionCheckbox";

type TemplateOption = {
	readonly id: string;
	readonly label: string;
	readonly activeVersionId: string;
};

type AgentParameterRulesDialogProps = {
	readonly template: TemplateOption;
	/** The rules of this template. */
	readonly rules: readonly ChatProjectTemplateParameterRule[];
	readonly isSaving: boolean;
	readonly error: unknown;
	readonly onClose: () => void;
	readonly onSave: (rules: ChatProjectTemplateParameterRule[]) => void;
};

/** Limits the parameter values chats may choose for one template. */
export const AgentParameterRulesDialog: React.FC<
	AgentParameterRulesDialogProps
> = ({ template, rules: initialRules, isSaving, error, onClose, onSave }) => {
	const form = useFormik<{ rules: ChatProjectTemplateParameterRule[] }>({
		initialValues: { rules: [...initialRules] },
		onSubmit: (values) => onSave(values.rules),
	});
	const parametersQuery = useQuery(richParameters(template.activeVersionId));
	const hasEmptyRule = form.values.rules.some(
		(r) => r.allowed_values.length === 0,
	);

	return (
		<Dialog
			open
			onOpenChange={(open) => {
				if (!open && !isSaving) {
					onClose();
				}
			}}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Parameters for {template.label}</DialogTitle>
					<DialogDescription>
						Limit the values chats in this agent can choose when they create a
						workspace from this template. Parameters you leave unrestricted can
						take any value.
					</DialogDescription>
				</DialogHeader>
				<form className="flex flex-col gap-4" onSubmit={form.handleSubmit}>
					<ParametersPanel
						parameters={parametersQuery.data}
						error={parametersQuery.error}
						rules={form.values.rules}
						onChange={(name, values) =>
							form.setFieldValue(
								"rules",
								withRule(form.values.rules, template.id, name, values),
							)
						}
					/>
					{hasEmptyRule && (
						<p className="m-0 text-sm text-content-destructive">
							Each restricted parameter needs at least one allowed value.
						</p>
					)}
					{Boolean(error) && <ErrorAlert error={error} />}
					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							disabled={isSaving}
							onClick={onClose}
						>
							Cancel
						</Button>
						<Button type="submit" disabled={isSaving || hasEmptyRule}>
							<Spinner loading={isSaving} />
							Save
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
};

const withRule = (
	rules: readonly ChatProjectTemplateParameterRule[],
	templateId: string,
	name: string,
	values: string[] | undefined,
): ChatProjectTemplateParameterRule[] => {
	const isTarget = (r: ChatProjectTemplateParameterRule) =>
		r.parameter_name === name;
	if (values === undefined) {
		return rules.filter((r) => !isTarget(r));
	}
	const rule = {
		template_id: templateId,
		parameter_name: name,
		allowed_values: values,
	};
	return rules.some(isTarget)
		? rules.map((r) => (isTarget(r) ? rule : r))
		: [...rules, rule];
};

type ParametersPanelProps = Omit<ParameterRulesEditorProps, "parameters"> & {
	readonly parameters: readonly TemplateVersionParameter[] | undefined;
	readonly error: unknown;
};

const ParametersPanel: React.FC<ParametersPanelProps> = ({
	parameters,
	error,
	rules,
	onChange,
}) => {
	if (error) {
		return <ErrorAlert error={error} />;
	}
	if (!parameters) {
		return <Spinner loading />;
	}
	return (
		<ParameterRulesEditor
			parameters={parameters}
			rules={rules}
			onChange={onChange}
		/>
	);
};

type ParameterRulesEditorProps = {
	readonly parameters: readonly TemplateVersionParameter[];
	/** Rules for the template the parameters belong to. */
	readonly rules: readonly ChatProjectTemplateParameterRule[];
	/** Pass undefined to remove the rule. */
	readonly onChange: (name: string, values: string[] | undefined) => void;
};

const ParameterRulesEditor: React.FC<ParameterRulesEditorProps> = ({
	parameters,
	rules,
	onChange,
}) => {
	const missing = rules.filter(
		(r) => !parameters.some((p) => p.name === r.parameter_name),
	);
	if (parameters.length === 0 && missing.length === 0) {
		return (
			<p className="m-0 text-sm text-content-secondary">
				This template has no parameters.
			</p>
		);
	}
	return (
		<div className="flex flex-col gap-4">
			{parameters.map((parameter) => (
				<ParameterRuleField
					key={parameter.name}
					parameter={parameter}
					values={
						rules.find((r) => r.parameter_name === parameter.name)
							?.allowed_values
					}
					onChange={(values) => onChange(parameter.name, values)}
				/>
			))}
			{missing.map((rule) => (
				<div
					key={rule.parameter_name}
					className="flex items-center justify-between gap-2 text-sm text-content-secondary"
				>
					<span>
						{rule.parameter_name} is no longer on this template's active
						version.
					</span>
					<Button
						type="button"
						size="sm"
						variant="subtle"
						onClick={() => onChange(rule.parameter_name, undefined)}
					>
						<XIcon />
						Remove
					</Button>
				</div>
			))}
		</div>
	);
};

type ParameterRuleFieldProps = {
	readonly parameter: TemplateVersionParameter;
	/** Allowed values with the default first, or undefined when unrestricted. */
	readonly values: readonly string[] | undefined;
	readonly onChange: (values: string[] | undefined) => void;
};

const ParameterRuleField: React.FC<ParameterRuleFieldProps> = ({
	parameter,
	values,
	onChange,
}) => {
	const label = parameter.display_name || parameter.name;
	const options = parameterOptions(parameter);
	const isRestricted = values !== undefined;

	const startRestricting = () => {
		const initial =
			parameter.default_value &&
			(!options || options.some((o) => o.value === parameter.default_value))
				? [parameter.default_value]
				: [];
		onChange(initial);
	};

	return (
		<fieldset className="m-0 flex flex-col gap-2 border-0 p-0">
			<legend className="sr-only">{label}</legend>
			<OptionCheckbox
				label={`Restrict ${label}`}
				checked={isRestricted}
				onCheckedChange={(checked) =>
					checked ? startRestricting() : onChange(undefined)
				}
			/>
			{isRestricted && (
				<div className="ml-6 flex flex-col gap-2">
					{options ? (
						options.map((option) => (
							<OptionCheckbox
								key={option.value}
								label={option.label}
								checked={values.includes(option.value)}
								onCheckedChange={(checked) =>
									onChange(
										checked
											? [...values, option.value]
											: values.filter((v) => v !== option.value),
									)
								}
							/>
						))
					) : (
						<TagInput
							label={`Allowed values for ${label}`}
							values={[...values]}
							onChange={(next) => onChange(uniqueNonEmpty(next))}
						/>
					)}
					{values.length > 1 && !values.includes("") && (
						<div className="flex items-center gap-2 text-sm">
							<span className="text-content-secondary">Default</span>
							<Select
								value={values[0]}
								onValueChange={(value) =>
									onChange([value, ...values.filter((v) => v !== value)])
								}
							>
								<SelectTrigger
									aria-label={`Default value for ${label}`}
									className="w-auto min-w-32"
								>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{values.map((v) => (
										<SelectItem key={v} value={v}>
											{options?.find((o) => o.value === v)?.label ?? v}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
					)}
				</div>
			)}
		</fieldset>
	);
};

const parameterOptions = (
	parameter: TemplateVersionParameter,
): { label: string; value: string }[] | undefined => {
	if (parameter.options.length > 0) {
		return parameter.options.map((o) => ({ label: o.name, value: o.value }));
	}
	if (parameter.type === "bool") {
		return [
			{ label: "true", value: "true" },
			{ label: "false", value: "false" },
		];
	}
	return undefined;
};

const uniqueNonEmpty = (values: readonly string[]): string[] => [
	...new Set(values.map((v) => v.trim()).filter((v) => v !== "")),
];
