import type { FormikContextType } from "formik";
import { InfoIcon } from "lucide-react";
import type { FC } from "react";
import { Input } from "#/components/Input/Input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Switch } from "#/components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { Field } from "./MCPServerFormFieldPrimitives";
import {
	AVAILABILITY_OPTIONS,
	type MCPServerFormValues,
} from "./mcpServerFormLogic";

interface MCPServerBehaviorSectionProps {
	form: FormikContextType<MCPServerFormValues>;
	formId: string;
	disabled: boolean;
}

export const MCPServerBehaviorSection: FC<MCPServerBehaviorSectionProps> = ({
	form,
	formId,
	disabled,
}) => {
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
					disabled={disabled}
					tooltip="Only enable for first-party or trusted MCP servers."
				/>
			</div>
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

const SwitchField: FC<{
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
