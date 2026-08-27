import type { FormikContextType } from "formik";
import { PlusIcon, XIcon } from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Switch } from "#/components/Switch/Switch";
import { Field } from "./MCPServerFormFieldPrimitives";
import {
	getMCPServerToolRuleErrors,
	type MCPServerFormValues,
} from "./mcpServerFormLogic";

const TOOL_DEFAULT_OPTIONS = [
	{ value: "enabled", label: "Enabled" },
	{ value: "disabled", label: "Disabled" },
] as const;

interface MCPServerToolRulesSectionProps {
	form: FormikContextType<MCPServerFormValues>;
	formId: string;
	disabled: boolean;
}

export const MCPServerToolRulesSection: FC<MCPServerToolRulesSectionProps> = ({
	form,
	formId,
	disabled,
}) => {
	const toolRuleErrors = getMCPServerToolRuleErrors(form.values.toolRules);
	const setToolRules = (toolRules: MCPServerFormValues["toolRules"]) =>
		void form.setFieldValue("toolRules", toolRules);

	return (
		<div className="space-y-5">
			<p className="m-0 text-sm text-content-secondary">
				An exact tool name rule overrides the server default. Tools without a
				matching rule use the server default. The legacy allow and deny regex
				lists in Behavior also apply, so a tool must pass both controls.
			</p>
			<Field
				label="Default tool state"
				htmlFor={`${formId}-tool-default`}
				className="max-w-md"
			>
				<Select
					value={form.values.toolDefault}
					onValueChange={(value) => {
						if (value === "enabled" || value === "disabled") {
							void form.setFieldValue("toolDefault", value);
						}
					}}
					disabled={disabled}
				>
					<SelectTrigger id={`${formId}-tool-default`} className="shadow-none">
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{TOOL_DEFAULT_OPTIONS.map((option) => (
							<SelectItem key={option.value} value={option.value}>
								{option.label}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</Field>

			<div className="space-y-3">
				{form.values.toolRules.length === 0 && (
					<p className="m-0 text-sm text-content-secondary">
						No per-tool rules. All tools use the server default.
					</p>
				)}
				{form.values.toolRules.map((rule, index) => {
					const toolNameId = `${formId}-tool-rule-${index}-name`;
					const toolNameErrorId = `${toolNameId}-error`;
					const enabledId = `${formId}-tool-rule-${index}-enabled`;
					const error = toolRuleErrors[index];

					return (
						<fieldset
							key={index.toString()}
							className="m-0 min-w-0 rounded-lg border border-solid border-border-default p-4"
						>
							<legend className="px-2 text-sm font-medium">
								Rule {index + 1}
							</legend>
							<div className="grid items-start gap-4 sm:grid-cols-[minmax(0,1fr)_auto_auto]">
								<Field label="Tool name" htmlFor={toolNameId} required>
									<Input
										id={toolNameId}
										className="placeholder:text-content-disabled shadow-none"
										value={rule.tool}
										placeholder="e.g. search"
										required
										aria-invalid={Boolean(error)}
										aria-describedby={error ? toolNameErrorId : undefined}
										onChange={(event) =>
											setToolRules(
												form.values.toolRules.map((currentRule, ruleIndex) =>
													ruleIndex === index
														? { ...currentRule, tool: event.target.value }
														: currentRule,
												),
											)
										}
										disabled={disabled}
									/>
									{error && (
										<p
											id={toolNameErrorId}
											className="m-0 text-xs text-content-destructive"
											role="alert"
										>
											{error}
										</p>
									)}
								</Field>
								<Field label="Enabled" htmlFor={enabledId}>
									<Switch
										id={enabledId}
										checked={rule.enabled}
										onCheckedChange={(enabled) =>
											setToolRules(
												form.values.toolRules.map((currentRule, ruleIndex) =>
													ruleIndex === index
														? { ...currentRule, enabled }
														: currentRule,
												),
											)
										}
										disabled={disabled}
									/>
								</Field>
								<Button
									variant="subtle"
									size="icon"
									className="sm:mt-7"
									aria-label={`Remove rule ${index + 1}`}
									onClick={() =>
										setToolRules(
											form.values.toolRules.filter(
												(_, ruleIndex) => ruleIndex !== index,
											),
										)
									}
									disabled={disabled}
								>
									<XIcon />
								</Button>
							</div>
						</fieldset>
					);
				})}
				<Button
					variant="outline"
					onClick={() =>
						setToolRules([
							...form.values.toolRules,
							{ tool: "", enabled: true },
						])
					}
					disabled={disabled}
				>
					<PlusIcon />
					Add rule
				</Button>
			</div>
		</div>
	);
};
