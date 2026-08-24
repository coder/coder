import {
	type FC,
	type FocusEventHandler,
	type ReactNode,
	useId,
	useState,
} from "react";
import type { TemplateVersionVariable } from "#/api/typesGenerated";
import { FormField } from "#/components/FormField/FormField";
import { Label } from "#/components/Label/Label";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import { cn } from "#/utils/cn";

export const SensitiveVariableHelperText: FC = () => {
	return (
		<span>
			This variable is sensitive. The previous value will be used if empty.
		</span>
	);
};

interface TemplateVariableFieldProps {
	templateVersionVariable: TemplateVersionVariable;
	initialValue: string;
	disabled: boolean;
	onChange: (value: string) => void;
	error?: boolean;
	helperText?: ReactNode;
	name?: string;
	onBlur?: FocusEventHandler<HTMLInputElement | HTMLTextAreaElement>;
}

export const TemplateVariableField: FC<TemplateVariableFieldProps> = ({
	templateVersionVariable,
	initialValue,
	disabled,
	onChange,
	error = false,
	helperText,
	name,
	onBlur,
}) => {
	const id = useId();
	const [variableValue, setVariableValue] = useState(initialValue);

	if (isBoolean(templateVersionVariable)) {
		const trueId = `${id}-true`;
		const falseId = `${id}-false`;
		const helperId = `${id}-helper`;

		return (
			<div className="flex flex-col gap-2">
				<RadioGroup
					value={variableValue}
					disabled={disabled}
					aria-invalid={error}
					aria-describedby={helperText ? helperId : undefined}
					onValueChange={(value) => {
						setVariableValue(value);
						onChange(value);
					}}
				>
					<div className="flex items-center gap-2">
						<RadioGroupItem id={trueId} value="true" />
						<Label htmlFor={trueId} className="font-normal cursor-pointer">
							True
						</Label>
					</div>
					<div className="flex items-center gap-2">
						<RadioGroupItem id={falseId} value="false" />
						<Label htmlFor={falseId} className="font-normal cursor-pointer">
							False
						</Label>
					</div>
				</RadioGroup>
				{helperText && (
					<span
						id={helperId}
						className={cn(
							"text-xs",
							error ? "text-content-destructive" : "text-content-secondary",
						)}
					>
						{helperText}
					</span>
				)}
			</div>
		);
	}

	return (
		<FormField
			field={{
				name: name ?? templateVersionVariable.name,
				id,
				value: variableValue,
				onChange: (event) => {
					setVariableValue(event.target.value);
					onChange(event.target.value);
				},
				onBlur: onBlur ?? (() => {}),
				error,
				helperText,
			}}
			label={templateVersionVariable.name}
			type={
				templateVersionVariable.type === "number"
					? "number"
					: templateVersionVariable.sensitive
						? "password"
						: "text"
			}
			disabled={disabled}
			autoFocus
			placeholder={
				templateVersionVariable.sensitive
					? ""
					: templateVersionVariable.default_value
			}
			className="w-full"
		/>
	);
};

const isBoolean = (variable: TemplateVersionVariable) => {
	return variable.type === "bool";
};
