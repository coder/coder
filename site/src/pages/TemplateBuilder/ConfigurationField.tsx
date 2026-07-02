import type { FC, PropsWithChildren, ReactNode } from "react";
import type { TemplateBuilderModuleVariable } from "#/api/typesGenerated";
import { FormField } from "#/components/FormField/FormField";
import { Label } from "#/components/Label/Label";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Switch } from "#/components/Switch/Switch";
import type { FormHelpers } from "#/utils/formUtils";

type FieldOption = {
	value: string;
	label: string;
	iconUrl?: string;
};

type SwitchItem = {
	id: string;
	label: string;
	defaultChecked?: boolean;
};

type BaseField = {
	id: string;
	label: ReactNode;
	description?: ReactNode;
	required?: boolean;
};

type TextFieldDefinition = BaseField & {
	type: "text";
	field: FormHelpers;
	placeholder?: string;
};

type SelectFieldDefinition = BaseField & {
	type: "select";
	value?: string;
	onChange?: (value: string) => void;
	placeholder?: string;
	options: FieldOption[];
};

type RadioFieldDefinition = BaseField & {
	type: "radio";
	value?: string;
	onChange?: (value: string) => void;
	options: FieldOption[];
};

type SwitchFieldDefinition = BaseField & {
	type: "switch";
	checked?: boolean;
	defaultChecked?: boolean;
	onCheckedChange?: (checked: boolean) => void;
};

type SwitchGroupFieldDefinition = BaseField & {
	type: "switch-group";
	switches: SwitchItem[];
};

export type ConfigurationFieldDefinition =
	| TextFieldDefinition
	| SelectFieldDefinition
	| RadioFieldDefinition
	| SwitchFieldDefinition
	| SwitchGroupFieldDefinition;

export const ConfigurationField: FC<{
	field: ConfigurationFieldDefinition;
}> = ({ field }) => {
	switch (field.type) {
		case "text":
			return <TextField {...field} />;
		case "select":
			return <SelectField {...field} />;
		case "radio":
			return <RadioField {...field} />;
		case "switch":
			return <SwitchField {...field} />;
		case "switch-group":
			return <SwitchGroupField {...field} />;
	}
};

const TextField: FC<TextFieldDefinition> = ({
	id,
	field,
	label,
	description,
	required,
	placeholder,
}) => (
	<FormField
		id={id}
		field={field}
		label={label}
		description={description}
		required={required}
		placeholder={placeholder}
	/>
);

const SelectField: FC<SelectFieldDefinition> = ({
	id,
	label,
	description,
	required,
	value,
	onChange,
	placeholder,
	options,
}) => {
	const descriptionId = `${id}-description`;
	return (
		// All fields span 2 columns, except for dropdowns which can only be 1 column (50% width)
		<div className="!col-end-1 flex flex-col gap-2">
			<Label htmlFor={id}>
				{label}
				{required ? (
					<>
						{" "}
						<span className="text-sm font-bold text-content-destructive">
							*
						</span>
					</>
				) : (
					<OptionalIndicator />
				)}
			</Label>
			{description && (
				<div id={descriptionId} className="text-sm text-content-secondary">
					{description}
				</div>
			)}
			<Select value={value} onValueChange={onChange}>
				<SelectTrigger
					id={id}
					aria-describedby={description ? descriptionId : undefined}
				>
					<SelectValue placeholder={placeholder ?? "Select..."} />
				</SelectTrigger>
				<SelectContent>
					{options.map((option) => (
						<SelectItem key={option.value} value={option.value}>
							{option.label}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</div>
	);
};

const RadioField: FC<RadioFieldDefinition> = ({
	id,
	label,
	description,
	required,
	value,
	onChange,
	options,
}) => {
	const labelId = `${id}-label`;
	const descriptionId = `${id}-description`;
	return (
		<div className="flex flex-col gap-2">
			<Label id={labelId}>
				{label}
				{required ? (
					<>
						{" "}
						<span className="text-sm font-bold text-content-destructive">
							*
						</span>
					</>
				) : (
					<OptionalIndicator />
				)}
			</Label>
			{description && (
				<div id={descriptionId} className="text-sm text-content-secondary">
					{description}
				</div>
			)}
			<RadioGroup
				value={value}
				onValueChange={onChange}
				aria-labelledby={labelId}
				aria-describedby={description ? descriptionId : undefined}
			>
				{options.map((option) => {
					const optionId = `${id}-${option.value}`;
					return (
						<div key={option.value} className="flex items-center gap-2">
							<RadioGroupItem id={optionId} value={option.value} />
							<Label
								htmlFor={optionId}
								className="flex items-center gap-1 font-normal"
							>
								{option.iconUrl && (
									<img
										src={option.iconUrl}
										alt=""
										className="size-6 object-contain"
									/>
								)}
								{option.label}
							</Label>
						</div>
					);
				})}
			</RadioGroup>
		</div>
	);
};

const SwitchRow: FC<{
	id: string;
	label: ReactNode;
	checked?: boolean;
	defaultChecked?: boolean;
	onCheckedChange?: (checked: boolean) => void;
	describedBy?: string;
}> = ({ id, label, checked, defaultChecked, onCheckedChange, describedBy }) => (
	<div className="flex items-center gap-2">
		<Switch
			id={id}
			checked={checked}
			defaultChecked={defaultChecked}
			onCheckedChange={onCheckedChange}
			aria-describedby={describedBy}
		/>
		<Label htmlFor={id}>{label}</Label>
	</div>
);

const SwitchField: FC<SwitchFieldDefinition> = ({
	id,
	label,
	description,
	required,
	checked,
	defaultChecked,
	onCheckedChange,
}) => {
	const descriptionId = `${id}-description`;
	return (
		<div className="flex flex-col gap-1">
			<SwitchRow
				id={id}
				label={
					<>
						{label}
						{required && (
							<>
								{" "}
								<span className="text-sm font-bold text-content-destructive">
									*
								</span>
							</>
						)}
					</>
				}
				checked={checked}
				defaultChecked={defaultChecked}
				onCheckedChange={onCheckedChange}
				describedBy={description ? descriptionId : undefined}
			/>
			{description && (
				<div
					id={descriptionId}
					className="ml-[44px] text-sm font-normal text-content-secondary"
				>
					{description}
				</div>
			)}
		</div>
	);
};

const SwitchGroupField: FC<SwitchGroupFieldDefinition> = ({
	id,
	label,
	description,
	required,
	switches,
}) => {
	const labelId = `${id}-label`;
	const descriptionId = `${id}-description`;
	return (
		<div className="flex flex-col gap-2">
			<Label id={labelId}>
				{label}
				{required ? (
					<>
						{" "}
						<span className="text-sm font-bold text-content-destructive">
							*
						</span>
					</>
				) : (
					<OptionalIndicator />
				)}
			</Label>
			{description && (
				<div id={descriptionId} className="text-sm text-content-secondary">
					{description}
				</div>
			)}
			<div
				className="flex flex-col gap-2"
				role="group"
				aria-labelledby={labelId}
				aria-describedby={description ? descriptionId : undefined}
			>
				{switches.map((item) => (
					<SwitchRow
						key={item.id}
						id={item.id}
						label={item.label}
						defaultChecked={item.defaultChecked}
					/>
				))}
			</div>
		</div>
	);
};

export const ConfigurationFieldContainer: FC<PropsWithChildren> = ({
	children,
}) => {
	return (
		<div className="grid grid-cols-1 md:grid-cols-2 gap-6 items-start *:col-start-1 *:col-span-full">
			{children}
		</div>
	);
};

const OptionalIndicator: FC = () => {
	return (
		<>
			{" "}
			<span className="text-content-secondary">(optional)</span>
		</>
	);
};

export const ConfigurationFieldLabel: FC<{
	variable: TemplateBuilderModuleVariable;
}> = ({ variable }) => {
	return (
		<>
			{variable.name}
			{!variable.required && <OptionalIndicator />}
		</>
	);
};
