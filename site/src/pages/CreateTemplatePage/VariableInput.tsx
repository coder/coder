import type { FC } from "react";
import type { TemplateVersionVariable } from "#/api/typesGenerated";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";

const isBoolean = (variable: TemplateVersionVariable) => {
	return variable.type === "bool";
};

interface VariableLabelProps {
	variable: TemplateVersionVariable;
}

const VariableLabel: FC<VariableLabelProps> = ({ variable }) => {
	return (
		<label htmlFor={variable.name}>
			<span className="mb-1 block text-sm text-content-secondary">
				var.{variable.name}
				{!variable.required && " (optional)"}
			</span>
			<span className="block text-base font-semibold text-content-primary">
				{variable.description}
			</span>
		</label>
	);
};

interface VariableInputProps {
	disabled?: boolean;
	variable: TemplateVersionVariable;
	onChange: (value: string) => void;
	defaultValue?: string;
}

export const VariableInput: FC<VariableInputProps> = ({
	disabled,
	onChange,
	variable,
	defaultValue,
}) => {
	return (
		<div className="flex flex-col gap-1.5">
			<VariableLabel variable={variable} />
			<div className="flex flex-col">
				<VariableField
					disabled={disabled}
					onChange={onChange}
					variable={variable}
					defaultValue={defaultValue}
				/>
			</div>
		</div>
	);
};

const VariableField: FC<VariableInputProps> = ({
	disabled,
	onChange,
	variable,
	defaultValue,
}) => {
	if (isBoolean(variable)) {
		const trueId = `${variable.name}-true`;
		const falseId = `${variable.name}-false`;

		return (
			<RadioGroup
				id={variable.name}
				defaultValue={variable.default_value}
				disabled={disabled}
				onValueChange={onChange}
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
		);
	}

	return (
		<Input
			autoComplete="off"
			id={variable.name}
			disabled={disabled}
			placeholder={variable.sensitive ? "" : variable.default_value}
			required={variable.required}
			defaultValue={
				variable.sensitive ? "" : (defaultValue ?? variable.default_value)
			}
			onChange={(event) => {
				onChange(event.target.value);
			}}
			type={
				variable.type === "number"
					? "number"
					: variable.sensitive
						? "password"
						: "text"
			}
		/>
	);
};
