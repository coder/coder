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
import { Field } from "./MCPServerFormFieldPrimitives";
import {
	getMCPServerToolRuleErrors,
	isToolDisposition,
	type MCPServerFormValues,
	TOOL_DISPOSITION_OPTIONS,
	toolRuleAction,
} from "./mcpServerFormLogic";

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
				matching rule use the server default. Escalated tools stay visible, but
				each call is held until the sponsoring user approves it. The legacy
				allow and deny regex lists in Behavior also apply, so a tool must pass
				both controls.
			</p>
			<Field
				label="Default tool state"
				htmlFor={`${formId}-tool-default`}
				className="max-w-md"
			>
				<Select
					value={form.values.toolDefault}
					onValueChange={(value) => {
						if (isToolDisposition(value)) {
							void form.setFieldValue("toolDefault", value);
						}
					}}
					disabled={disabled}
				>
					<SelectTrigger id={`${formId}-tool-default`} className="shadow-none">
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{TOOL_DISPOSITION_OPTIONS.map((option) => (
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
					const actionId = `${formId}-tool-rule-${index}-action`;
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
								<Field label="Action" htmlFor={actionId}>
									<Select
										value={toolRuleAction(rule)}
										onValueChange={(value) => {
											if (!isToolDisposition(value)) {
												return;
											}
											setToolRules(
												form.values.toolRules.map((currentRule, ruleIndex) =>
													ruleIndex === index
														? {
																...currentRule,
																action: value,
																enabled: value === "enabled",
															}
														: currentRule,
												),
											);
										}}
										disabled={disabled}
									>
										<SelectTrigger id={actionId} className="w-56 shadow-none">
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											{TOOL_DISPOSITION_OPTIONS.map((option) => (
												<SelectItem key={option.value} value={option.value}>
													{option.label}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
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
