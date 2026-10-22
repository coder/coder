import type { FormikContextType } from "formik";
import { CheckIcon, InfoIcon } from "lucide-react";
import { Select as SelectPrimitive } from "radix-ui";
import { Input } from "#/components/Input/Input";
import {
	Select,
	SelectContent,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Switch } from "#/components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { SecretInput } from "./MCPServerAuthSection";
import { Field } from "./MCPServerFormFieldPrimitives";
import {
	AVAILABILITY_FIELD_DESCRIPTION,
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
	return (
		<>
			<Field
				label="Availability in chats"
				htmlFor={`${formId}-availability`}
				className="max-w-md"
				description={AVAILABILITY_FIELD_DESCRIPTION}
			>
				<Select
					value={form.values.availability}
					onValueChange={(value) =>
						void form.setFieldValue("availability", value)
					}
					disabled={disabled}
				>
					<SelectTrigger
						id={`${formId}-availability`}
						className="shadow-none"
						aria-describedby={`${formId}-availability-help`}
					>
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{AVAILABILITY_OPTIONS.map((option) => (
							<SelectPrimitive.Item
								key={option.value}
								value={option.value}
								className="relative flex w-full cursor-default select-none items-start rounded-sm py-1.5 pl-2 pr-8 text-sm text-content-secondary outline-hidden focus:bg-surface-secondary focus:text-content-primary data-disabled:pointer-events-none data-disabled:opacity-50"
							>
								<span className="absolute right-2 top-2 flex items-center justify-center">
									<SelectPrimitive.ItemIndicator>
										<CheckIcon className="size-icon-sm" />
									</SelectPrimitive.ItemIndicator>
								</span>
								<div className="flex flex-col py-0.5">
									<SelectPrimitive.ItemText>
										{option.label}
									</SelectPrimitive.ItemText>
									<span className="text-xs text-content-secondary whitespace-normal wrap-break-word">
										{option.description}
									</span>
								</div>
							</SelectPrimitive.Item>
						))}
					</SelectContent>
				</Select>
				<p
					id={`${formId}-availability-help`}
					className="m-0 text-xs text-content-secondary"
				>
					{
						AVAILABILITY_OPTIONS.find(
							(option) => option.value === form.values.availability,
						)?.helpText
					}
				</p>
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
					disabled={disabled}
					tooltip="Only enable for first-party or trusted MCP servers."
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

const SwitchField: React.FC<{
	label: string;
	checked: boolean;
	onCheckedChange: (checked: boolean) => void;
	disabled: boolean;
	tooltip: string;
}> = ({ label, checked, onCheckedChange, disabled, tooltip }) => (
	<div className="flex items-center gap-3">
		<Switch
			checked={checked}
			onCheckedChange={onCheckedChange}
			disabled={disabled}
			aria-label={label}
		/>
		<span className="text-sm text-content-primary">{label}</span>
		<Tooltip>
			<TooltipTrigger asChild>
				<InfoIcon className="size-3 text-content-secondary" />
			</TooltipTrigger>
			<TooltipContent side="top" className="max-w-[260px]">
				{tooltip}
			</TooltipContent>
		</Tooltip>
	</div>
);
