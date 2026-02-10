import type { TemplateVersionVariable } from "api/typesGenerated";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { RadioGroup, RadioGroupItem } from "components/RadioGroup/RadioGroup";
import { type FC, useState } from "react";

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
}

export const TemplateVariableField: FC<TemplateVariableFieldProps> = ({
	templateVersionVariable,
	initialValue,
	disabled,
	onChange,
}) => {
	const [variableValue, setVariableValue] = useState(initialValue);
	if (isBoolean(templateVersionVariable)) {
		return (
			<RadioGroup
				defaultValue={variableValue}
				onValueChange={(value) => {
					onChange(value);
				}}
				disabled={disabled}
			>
				<div className="flex items-center gap-2">
					<RadioGroupItem value="true" id="radio-true" />
					<Label htmlFor="radio-true">True</Label>
				</div>
				<div className="flex items-center gap-2">
					<RadioGroupItem value="false" id="radio-false" />
					<Label htmlFor="radio-false">False</Label>
				</div>
			</RadioGroup>
		);
	}

	return (
		<div className="flex flex-col gap-2">
			<Label htmlFor={`var-${templateVersionVariable.name}`}>
				{templateVersionVariable.name}
			</Label>
			<Input
				id={`var-${templateVersionVariable.name}`}
				type={
					templateVersionVariable.type === "number"
						? "number"
						: templateVersionVariable.sensitive
							? "password"
							: "text"
				}
				disabled={disabled}
				autoFocus
				className="w-full"
				value={variableValue}
				placeholder={
					templateVersionVariable.sensitive
						? ""
						: templateVersionVariable.default_value
				}
				onChange={(event) => {
					setVariableValue(event.target.value);
					onChange(event.target.value);
				}}
			/>
		</div>
	);
};

const isBoolean = (variable: TemplateVersionVariable) => {
	return variable.type === "bool";
};
