import FormControlLabel from "@mui/material/FormControlLabel";
import Radio from "@mui/material/Radio";
import RadioGroup from "@mui/material/RadioGroup";
import TextField from "@mui/material/TextField";
import type { FC } from "react";
import type { TemplateVersionVariable } from "#/api/typesGenerated";

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
		return (
			<RadioGroup
				id={variable.name}
				defaultValue={variable.default_value}
				onChange={(event) => {
					onChange(event.target.value);
				}}
			>
				<FormControlLabel
					disabled={disabled}
					value="true"
					control={<Radio size="small" />}
					label="True"
				/>
				<FormControlLabel
					disabled={disabled}
					value="false"
					control={<Radio size="small" />}
					label="False"
				/>
			</RadioGroup>
		);
	}

	return (
		<TextField
			autoComplete="off"
			id={variable.name}
			size="small"
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
						: "string"
			}
		/>
	);
};
