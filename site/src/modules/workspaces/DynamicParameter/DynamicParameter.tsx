import type { Parameter, ParameterOption } from "api/typesParameter";
import { Badge } from "components/Badge/Badge";
import { Checkbox } from "components/Checkbox/Checkbox";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { MemoizedMarkdown } from "components/Markdown/Markdown";
import {
	MultiSelectCombobox,
	type Option,
} from "components/MultiSelectCombobox/MultiSelectCombobox";
import { RadioGroup, RadioGroupItem } from "components/RadioGroup/RadioGroup";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { Switch } from "components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { Info, Settings, TriangleAlert } from "lucide-react";
import { type FC, useId } from "react";

export interface DynamicParameterProps {
	parameter: Parameter;
	onChange: (value: string) => void;
	disabled?: boolean;
	isPreset?: boolean;
}

export const DynamicParameter: FC<DynamicParameterProps> = ({
	parameter,
	onChange,
	disabled,
	isPreset,
}) => {
	const id = useId();

	return (
		<div
			className="flex flex-col gap-2"
			data-testid={`parameter-field-${parameter.name}`}
		>
			<ParameterLabel parameter={parameter} isPreset={isPreset} />
			<ParameterField
				parameter={parameter}
				onChange={onChange}
				disabled={disabled}
				id={id}
			/>
			{parameter.diagnostics.length > 0 && (
				<ParameterDiagnostics diagnostics={parameter.diagnostics} />
			)}
		</div>
	);
};

interface ParameterLabelProps {
	parameter: Parameter;
	isPreset?: boolean;
}

const ParameterLabel: FC<ParameterLabelProps> = ({ parameter, isPreset }) => {
	const hasDescription = parameter.description && parameter.description !== "";
	const displayName = parameter.display_name
		? parameter.display_name
		: parameter.name;

	return (
		<div className="flex items-start gap-2">
			{parameter.icon && (
				<span className="w-5 h-5">
					<ExternalImage
						className="w-full h-full mt-0.5 object-contain"
						alt="Parameter icon"
						src={parameter.icon}
					/>
				</span>
			)}

			<div className="flex flex-col gap-1.5">
				<Label className="flex gap-2 flex-wrap text-sm font-medium">
					{displayName}

					{parameter.mutable && (
						<TooltipProvider delayDuration={100}>
							<Tooltip>
								<TooltipTrigger asChild>
									<span className="flex items-center">
										<Badge size="sm" variant="warning">
											<TriangleAlert />
											Immutable
										</Badge>
									</span>
								</TooltipTrigger>
								<TooltipContent className="max-w-xs">
									This value cannot be modified after the workspace has been
									created.
								</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					)}
					{isPreset && (
						<TooltipProvider delayDuration={100}>
							<Tooltip>
								<TooltipTrigger asChild>
									<span className="flex items-center">
										<Badge size="sm">
											<Settings />
											Preset
										</Badge>
									</span>
								</TooltipTrigger>
								<TooltipContent className="max-w-xs">
									Preset parameters cannot be modified.
								</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					)}
				</Label>

				{hasDescription && (
					<div className="text-content-secondary">
						<MemoizedMarkdown className="text-xs">
							{parameter.description}
						</MemoizedMarkdown>
					</div>
				)}
			</div>
		</div>
	);
};

interface ParameterFieldProps {
	parameter: Parameter;
	onChange: (value: string) => void;
	disabled?: boolean;
	id: string;
}

const ParameterField: FC<ParameterFieldProps> = ({
	parameter,
	onChange,
	disabled,
	id,
}) => {
	const value = parameter.value.valid ? parameter.value.value : "";
	const defaultValue = parameter.default_value.valid
		? parameter.default_value.value
		: "";

	switch (parameter.form_type) {
		case "dropdown":
			return (
				<Select
					onValueChange={onChange}
					defaultValue={defaultValue}
					disabled={disabled}
				>
					<SelectTrigger>
						<SelectValue placeholder="Select option" />
					</SelectTrigger>
					<SelectContent>
						{parameter.options.map((option) => (
							<SelectItem key={option.value.value} value={option.value.value}>
								<OptionDisplay option={option} />
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			);

		case "multi-select": {
			// Map parameter options to MultiSelectCombobox options format
			const comboboxOptions: Option[] = parameter.options.map((opt) => ({
				value: opt.value.value,
				label: opt.name,
				disable: false,
			}));

			const defaultOptions: Option[] = JSON.parse(defaultValue).map(
				(val: string) => {
					const option = parameter.options.find((o) => o.value.value === val);
					return {
						value: val,
						label: option?.name || val,
						disable: false,
					};
				},
			);

			return (
				<MultiSelectCombobox
					inputProps={{
						id: `${id}-${parameter.name}`,
					}}
					options={comboboxOptions}
					defaultOptions={defaultOptions}
					onChange={(newValues) => {
						const values = newValues.map((option) => option.value);
						onChange(JSON.stringify(values));
					}}
					hidePlaceholderWhenSelected
					placeholder="Select option"
					emptyIndicator={
						<p className="text-center text-md text-content-primary">
							No results found
						</p>
					}
					disabled={disabled}
				/>
			);
		}

		case "switch":
			return (
				<Switch
					checked={value === "true"}
					onCheckedChange={(checked) => {
						onChange(checked ? "true" : "false");
					}}
					disabled={disabled}
				/>
			);

		case "radio":
			return (
				<RadioGroup
					onValueChange={onChange}
					disabled={disabled}
					defaultValue={defaultValue}
				>
					{parameter.options.map((option) => (
						<div
							key={option.value.value}
							className="flex items-center space-x-2"
						>
							<RadioGroupItem
								id={option.value.value}
								value={option.value.value}
							/>
							<Label htmlFor={option.value.value} className="cursor-pointer">
								<OptionDisplay option={option} />
							</Label>
						</div>
					))}
				</RadioGroup>
			);

		case "checkbox":
			return (
				<div className="flex items-center space-x-2">
					<Checkbox
						id={parameter.name}
						checked={value === "true"}
						defaultChecked={defaultValue === "true"} // TODO: defaultChecked is always overridden by checked
						onCheckedChange={(checked) => {
							onChange(checked ? "true" : "false");
						}}
						disabled={disabled}
					/>
					<Label htmlFor={parameter.name}>
						{parameter.display_name || parameter.name}
					</Label>
				</div>
			);
		case "input": {
			const inputType = parameter.type === "number" ? "number" : "text";
			const inputProps: Record<string, unknown> = {};

			if (parameter.type === "number") {
				const validations = parameter.validations[0] || {};
				const { validation_min, validation_max } = validations;

				if (validation_min !== null) {
					inputProps.min = validation_min;
				}

				if (validation_max !== null) {
					inputProps.max = validation_max;
				}
			}

			return (
				<Input
					type={inputType}
					defaultValue={defaultValue}
					onChange={(e) => onChange(e.target.value)}
					disabled={disabled}
					placeholder={
						(parameter.styling as { placehholder?: string })?.placehholder
					}
					{...inputProps}
				/>
			);
		}
	}
};

interface OptionDisplayProps {
	option: ParameterOption;
}

const OptionDisplay: FC<OptionDisplayProps> = ({ option }) => {
	return (
		<div className="flex items-center gap-2">
			{option.icon && (
				<ExternalImage
					className="w-4 h-4 object-contain"
					src={option.icon}
					alt=""
				/>
			)}
			<span>{option.name}</span>
			{option.description && (
				<TooltipProvider delayDuration={100}>
					<Tooltip>
						<TooltipTrigger asChild>
							<Info className="w-3.5 h-3.5 text-content-secondary" />
						</TooltipTrigger>
						<TooltipContent>{option.description}</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			)}
		</div>
	);
};

interface ParameterDiagnosticsProps {
	diagnostics: Parameter["diagnostics"];
}

const ParameterDiagnostics: FC<ParameterDiagnosticsProps> = ({
	diagnostics,
}) => {
	return (
		<div className="flex flex-col gap-2">
			{diagnostics.map((diagnostic, index) => (
				<div
					key={`diagnostic-${diagnostic.summary}-${index}`}
					className={`text-xs px-1 ${
						diagnostic.severity === "error"
							? "text-content-destructive"
							: "text-content-warning"
					}`}
				>
					<div className="font-medium">{diagnostic.summary}</div>
					{diagnostic.detail && <div>{diagnostic.detail}</div>}
				</div>
			))}
		</div>
	);
};
