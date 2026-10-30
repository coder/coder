import type { FormikContextType } from "formik";
import { Input } from "#/components/Input/Input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { SecretInput } from "./MCPServerAuthSection";
import { Field, SwitchField } from "./MCPServerFormFieldPrimitives";
import {
	AVAILABILITY_OPTIONS,
	type MCPServerFormValues,
} from "./mcpServerFormLogic";

type MCPServerBehaviorSectionProps = {
	form: FormikContextType<MCPServerFormValues>;
	formId: string;
	disabled: boolean;
};

export const MCPServerBehaviorSection: React.FC<
	MCPServerBehaviorSectionProps
> = ({ form, formId, disabled }) => {
	const isBuiltIn = form.values.preset !== "custom";
	return (
		<>
			<Field
				label="Availability"
				htmlFor={`${formId}-availability`}
				className="max-w-md"
				description={
					AVAILABILITY_OPTIONS.find(
						(option) => option.value === form.values.availability,
					)?.description
				}
			>
				<Select
					value={form.values.availability}
					onValueChange={(value) =>
						void form.setFieldValue("availability", value)
					}
					disabled={disabled}
				>
					<SelectTrigger id={`${formId}-availability`} className="shadow-none">
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{AVAILABILITY_OPTIONS.map((option) => (
							<SelectItem key={option.value} value={option.value}>
								{option.label}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</Field>
			<div className="flex flex-col gap-4">
				<SwitchField
					label="Model intent"
					checked={form.values.modelIntent}
					onCheckedChange={(checked) =>
						void form.setFieldValue("modelIntent", checked)
					}
					disabled={disabled}
					tooltip="Allows this server to be used for model-intent tools."
				/>
				<SwitchField
					label="Allow all tools from this MCP server in root plan mode"
					checked={form.values.allowInPlanMode}
					onCheckedChange={(checked) =>
						void form.setFieldValue("allowInPlanMode", checked)
					}
					disabled={disabled}
					tooltip="Allows tools during planning. Workspace MCP and plan-mode controls still apply."
				/>
				<SwitchField
					label="Forward Coder identity headers"
					checked={form.values.forwardCoderHeaders}
					onCheckedChange={(checked) =>
						void form.setFieldValue("forwardCoderHeaders", checked)
					}
					// Built-in servers verify who is calling, so they cannot run
					// without the headers.
					disabled={disabled || (isBuiltIn && form.values.forwardCoderHeaders)}
					tooltip="Only enable for first-party or trusted MCP servers."
					description={
						isBuiltIn
							? "Built-in servers need the identity headers and a signing secret, so this setting stays on."
							: undefined
					}
				/>
			</div>
			{form.values.forwardCoderHeaders && (
				<Field
					label="Signing secret"
					htmlFor={`${formId}-signing-secret`}
					description="Configure the same random secret on the MCP server. Without a secret, requests are unsigned."
				>
					<SecretInput
						id={`${formId}-signing-secret`}
						value={form.values.signingSecret}
						touched={form.values.signingSecretTouched}
						onTouch={() =>
							void form.setFieldValue("signingSecretTouched", true)
						}
						onValueChange={(value) =>
							void form.setFieldValue("signingSecret", value)
						}
						onReset={() =>
							void form.setFieldValue("signingSecretTouched", false)
						}
						disabled={disabled}
					/>
				</Field>
			)}
			<div className="grid items-start gap-4 sm:grid-cols-2">
				<Field
					label="Tool allow list"
					htmlFor={`${formId}-allow-list`}
					description="Comma-separated. Empty = all allowed."
				>
					<Input
						id={`${formId}-allow-list`}
						className="placeholder:text-content-disabled shadow-none"
						{...form.getFieldProps("toolAllowList")}
						placeholder="tool 1, tool 2"
						disabled={disabled}
					/>
				</Field>
				<Field
					label="Tool deny list"
					htmlFor={`${formId}-deny-list`}
					description="Comma-separated names to block."
				>
					<Input
						id={`${formId}-deny-list`}
						className="placeholder:text-content-disabled shadow-none"
						{...form.getFieldProps("toolDenyList")}
						placeholder="tool 1, tool 2"
						disabled={disabled}
					/>
				</Field>
			</div>
		</>
	);
};
