import FormControlLabel from "@mui/material/FormControlLabel";
import Radio from "@mui/material/Radio";
import RadioGroup from "@mui/material/RadioGroup";
import TextField from "@mui/material/TextField";
import type { FC } from "react";
import type { TemplateVersionVariable } from "#/api/typesGenerated";
import { MemoizedMarkdown } from "#/components/Markdown/Markdown";

const isBoolean = (variable: TemplateVersionVariable) => {
	return variable.type === "bool";
};

interface VariableLabelProps {
	variable: TemplateVersionVariable;
}

const descriptionId = (variable: TemplateVersionVariable) =>
	`${variable.name}-description`;

const VariableLabel: FC<VariableLabelProps> = ({ variable }) => {
	return (
		<div className="flex flex-col">
			<label htmlFor={variable.name}>
				<span className="block text-sm text-content-secondary">
					var.{variable.name}
					{!variable.required && " (optional)"}
				</span>
			</label>
			{/*
			 * The description is rendered as Markdown and kept outside the <label>
			 * so interactive content like links is not nested inside it (which would
			 * steal clicks meant for the link to focus the field). It is associated
			 * with the field via aria-describedby instead.
			 */}
			{variable.description && (
				<div id={descriptionId(variable)}>
					<MemoizedMarkdown className="mt-1 text-base font-semibold text-content-primary">
						{variable.description}
					</MemoizedMarkdown>
				</div>
			)}
		</div>
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
				aria-describedby={
					variable.description ? descriptionId(variable) : undefined
				}
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
			aria-describedby={
				variable.description ? descriptionId(variable) : undefined
			}
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
