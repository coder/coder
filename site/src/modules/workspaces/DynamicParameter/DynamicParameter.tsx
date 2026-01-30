import type {
	NullHCLString,
	PreviewParameter,
	PreviewParameterOption,
	WorkspaceBuildParameter,
} from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { Checkbox } from "components/Checkbox/Checkbox";
import {
	Combobox,
	ComboboxButton,
	ComboboxContent,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "components/Combobox/Combobox";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { MemoizedMarkdown } from "components/Markdown/Markdown";
import {
	MultiSelectCombobox,
	type Option,
} from "components/MultiSelectCombobox/MultiSelectCombobox";
import { RadioGroup, RadioGroupItem } from "components/RadioGroup/RadioGroup";
import { Slider } from "components/Slider/Slider";
import { Stack } from "components/Stack/Stack";
import { Switch } from "components/Switch/Switch";
import { TagInput } from "components/TagInput/TagInput";
import { Textarea } from "components/Textarea/Textarea";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	CircleAlert,
	Eye,
	EyeOff,
	Hourglass,
	Info,
	LinkIcon,
	Settings,
	TriangleAlert,
} from "lucide-react";
import { type FC, useId, useRef, useState } from "react";
import { cn } from "utils/cn";
import type { AutofillBuildParameter } from "utils/richParameters";
import * as Yup from "yup";

interface DynamicParameterProps {
	parameter: PreviewParameter;
	value?: string;
	onChange: (value: string) => void;
	disabled?: boolean;
	isPreset?: boolean;
	autofill?: boolean;
}

export const DynamicParameter: FC<DynamicParameterProps> = ({
	parameter,
	value,
	onChange,
	disabled,
	isPreset,
	autofill = false,
}) => {
	const id = useId();

	return (
		<div
			className="flex flex-col gap-2"
			data-testid={`parameter-field-${parameter.name}`}
		>
			<ParameterLabel
				id={id}
				parameter={parameter}
				isPreset={isPreset}
				autofill={autofill}
			/>
			<div className="max-w-lg">
				<ParameterField
					id={id}
					parameter={parameter}
					value={value}
					onChange={onChange}
					disabled={disabled}
				/>
			</div>
			{parameter.form_type !== "error" && (
				<ParameterDiagnostics diagnostics={parameter.diagnostics} />
			)}
		</div>
	);
};

interface ParameterLabelProps {
	parameter: PreviewParameter;
	isPreset?: boolean;
	autofill: boolean;
	id: string;
}

const ParameterLabel: FC<ParameterLabelProps> = ({
	parameter,
	isPreset,
	autofill,
	id,
}) => {
	const displayName = parameter.display_name
		? parameter.display_name
		: parameter.name;
	const hasRequiredDiagnostic = parameter.diagnostics?.find(
		(d) => d.extra?.code === "required",
	);

	return (
		<div className="flex items-start gap-2">
			{parameter.icon && (
				<ExternalImage
					className="w-5 h-5 mt-0.5 object-contain"
					alt="Parameter icon"
					src={parameter.icon}
				/>
			)}

			<div className="flex flex-col w-full gap-1">
				<Label
					htmlFor={id}
					className="flex gap-2 flex-wrap text-sm font-medium"
				>
					<span className="flex font-semibold">
						{displayName}
						{parameter.required && (
							<span className="text-content-destructive">*</span>
						)}
					</span>
					{!parameter.mutable && (
						<TooltipProvider delayDuration={100}>
							<Tooltip>
								<TooltipTrigger asChild>
									<span className="flex items-center">
										<Badge size="sm" variant="warning" border="none">
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
					{parameter.ephemeral && (
						<TooltipProvider delayDuration={100}>
							<Tooltip>
								<TooltipTrigger asChild>
									<span className="flex items-center">
										<Badge size="sm" variant="green" border="none">
											<Hourglass />
											Ephemeral
										</Badge>
									</span>
								</TooltipTrigger>
								<TooltipContent className="max-w-xs">
									This parameter is ephemeral and will reset to the template
									default on workspace restart.
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
					{autofill && (
						<TooltipProvider delayDuration={100}>
							<Tooltip>
								<TooltipTrigger asChild>
									<span className="flex items-center">
										<Badge size="sm">
											<LinkIcon />
											URL Autofill
										</Badge>
									</span>
								</TooltipTrigger>
								<TooltipContent className="max-w-xs">
									Autofilled from the URL.
								</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					)}
					{hasRequiredDiagnostic && (
						<TooltipProvider delayDuration={100}>
							<Tooltip>
								<TooltipTrigger asChild>
									<span className="flex items-center">
										<Badge size="sm" variant="destructive" border="none">
											Required
										</Badge>
									</span>
								</TooltipTrigger>
								<TooltipContent className="max-w-xs">
									{hasRequiredDiagnostic.summary || "Required parameter"}
								</TooltipContent>
							</Tooltip>
						</TooltipProvider>
					)}
				</Label>

				{Boolean(parameter.description) && (
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
	parameter: PreviewParameter;
	value?: string;
	onChange: (value: string) => void;
	disabled?: boolean;
	id: string;
}

const ParameterField: FC<ParameterFieldProps> = ({
	parameter,
	value,
	onChange,
	disabled,
	id,
}) => {
	if (value === undefined && parameter.value.valid) {
		value = parameter.value.value;
	}

	switch (parameter.form_type) {
		case "textarea": {
			const maskInput = parameter.styling?.mask_input ?? false;

			return (
				<MaskableTextArea
					id={id}
					onChange={onChange}
					value={value}
					masked={maskInput}
					disabled={disabled}
					required={parameter.required}
					placeholder={parameter.styling?.placeholder}
				/>
			);
		}

		case "input": {
			let maskInput = parameter.styling?.mask_input ?? false;
			const inputProps: Partial<MaskableInputProps> = {
				type: "text",
			};

			if (parameter.type === "number") {
				// Only text can be effectively masked
				maskInput = false;

				inputProps.type = "number";

				const { validation_min, validation_max } =
					parameter.validations[0] ?? {};
				if (validation_min !== null) {
					inputProps.min = validation_min;
				}
				if (validation_max !== null) {
					inputProps.max = validation_max;
				}
			} else if (parameter.styling?.mask_input) {
				inputProps.type = "password";
			}

			return (
				<MaskableInput
					id={id}
					onChange={onChange}
					value={value}
					masked={maskInput}
					disabled={disabled}
					required={parameter.required}
					placeholder={parameter.styling?.placeholder}
					{...inputProps}
				/>
			);
		}

		case "slider": {
			const numericValue = Number.isFinite(Number(value)) ? Number(value) : 0;
			const { validation_min: min = 0, validation_max: max = 100 } =
				parameter.validations[0] ?? {};

			return (
				<div className="flex flex-row items-baseline gap-3">
					<Slider
						id={id}
						className="mt-2"
						value={[numericValue]}
						onValueChange={([value]) => {
							onChange(value.toString());
						}}
						min={min ?? undefined}
						max={max ?? undefined}
						disabled={disabled}
					/>
					<span className="w-4 font-medium">{numericValue}</span>
				</div>
			);
		}

		case "dropdown": {
			const selectedOption = parameter.options.find(
				(opt) => opt.value.value === value,
			);
			return (
				<Combobox>
					<ComboboxTrigger asChild>
						<ComboboxButton
							selectedOption={
								selectedOption
									? {
											label: selectedOption.name,
											value: selectedOption.value.value,
										}
									: undefined
							}
							placeholder={parameter.styling?.placeholder || "Select option"}
							disabled={disabled}
						/>
					</ComboboxTrigger>
					<ComboboxContent>
						<ComboboxList>
							{parameter.options.map((option) => (
								<ComboboxItem
									key={option.value.value}
									value={option.value.value}
									selectedOption={
										selectedOption
											? {
													label: selectedOption.name,
													value: selectedOption.value.value,
												}
											: undefined
									}
									onSelect={(selectedValue) => {
										onChange(selectedValue);
									}}
								>
									{option.name}
								</ComboboxItem>
							))}
						</ComboboxList>
					</ComboboxContent>
				</Combobox>
			);
		}

		case "multi-select": {
			const parsedValues = parseStringArrayValue(value ?? "");

			if (parsedValues.error) {
				// Diagnostics on parameter already handle this case, do not duplicate error message
				// Reset user's values to an empty array. This would overwrite any default values
				parsedValues.values = [];
			}

			// Map parameter options to MultiSelectCombobox options format
			const options: Option[] = parameter.options.map((opt) => ({
				value: opt.value.value,
				label: opt.name,
				icon: opt.icon,
				description: opt.description,
				disable: false,
			}));

			const optionMap = new Map(
				parameter.options.map((opt) => [opt.value.value, opt.name]),
			);

			const selectedOptions: Option[] = parsedValues.values.map((val) => {
				return {
					value: val,
					label: optionMap.get(val) || val,
					disable: false,
				};
			});

			return (
				<MultiSelectCombobox
					inputProps={{
						id: id,
					}}
					data-testid={`multiselect-${parameter.name}`}
					options={options}
					defaultOptions={selectedOptions}
					onChange={(newValues) => {
						const values = newValues.map((option) => option.value);
						onChange(JSON.stringify(values));
					}}
					hidePlaceholderWhenSelected
					placeholder={parameter.styling?.placeholder || "Select option"}
					emptyIndicator={
						<p className="text-center text-md text-content-primary">
							No results found
						</p>
					}
					disabled={disabled}
				/>
			);
		}

		case "tag-select": {
			const parsedValues = parseStringArrayValue(value ?? "");

			if (parsedValues.error) {
				// Diagnostics on parameter already handle this case, do not duplicate error message
				// Reset user's values to an empty array. This would overwrite any default values
				parsedValues.values = [];
			}

			return (
				<TagInput
					id={id}
					label={parameter.display_name || parameter.name}
					values={parsedValues.values}
					onChange={(values) => {
						onChange(JSON.stringify(values));
					}}
				/>
			);
		}

		case "switch":
			return (
				<Switch
					id={id}
					checked={value === "true"}
					onCheckedChange={(checked) => {
						onChange(checked ? "true" : "false");
					}}
					disabled={disabled}
				/>
			);

		case "radio":
			return (
				<RadioGroup onValueChange={onChange} disabled={disabled} value={value}>
					{parameter.options.map((option) => (
						<div
							key={option.value.value}
							className="flex items-center space-x-2"
						>
							<RadioGroupItem
								id={`${id}-${option.value.value}`}
								value={option.value.value}
							/>
							<Label
								htmlFor={`${id}-${option.value.value}`}
								className="cursor-pointer"
							>
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
						id={id}
						checked={value === "true"}
						onCheckedChange={(checked) => {
							onChange(checked ? "true" : "false");
						}}
						disabled={disabled}
					/>
					<Label htmlFor={id}>{parameter.styling?.label}</Label>
				</div>
			);

		case "error":
			return <Diagnostics diagnostics={parameter.diagnostics} />;
	}
};

type MaskableInputProps = Omit<React.ComponentProps<"input">, "onChange"> & {
	onChange: (value: string) => void;
	masked?: boolean;
};

const MaskableInput: FC<MaskableInputProps> = ({
	id,
	onChange,
	value,
	masked,
	disabled,
	required,
	placeholder,
	type,
	...inputProps
}) => {
	const [showMaskedInput, setShowMaskedInput] = useState(false);

	return (
		<Stack direction="row" spacing={0} alignItems="center">
			<Input
				id={id}
				type={masked && showMaskedInput ? "text" : type}
				value={value}
				onChange={(e) => {
					onChange(e.target.value);
				}}
				disabled={disabled}
				required={required}
				placeholder={placeholder}
				{...inputProps}
			/>
			{masked && (
				<Button
					type="button"
					variant="subtle"
					size="icon"
					onMouseDown={() => setShowMaskedInput(true)}
					onMouseOut={() => setShowMaskedInput(false)}
					onMouseUp={() => setShowMaskedInput(false)}
					disabled={disabled}
				>
					{showMaskedInput ? (
						<EyeOff className="h-4 w-4" />
					) : (
						<Eye className="h-4 w-4" />
					)}
				</Button>
			)}
		</Stack>
	);
};

const MaskableTextArea: FC<MaskableInputProps> = ({
	id,
	onChange,
	value,
	masked,
	disabled,
	placeholder,
	required,
}) => {
	const textareaRef = useRef<HTMLTextAreaElement>(null);
	const [showMaskedInput, setShowMaskedInput] = useState(false);

	return (
		<Stack direction="row" spacing={0} alignItems="center">
			<Textarea
				ref={textareaRef}
				id={id}
				className={cn(
					"overflow-y-auto max-h-[500px]",
					masked && !showMaskedInput && "[-webkit-text-security:disc]",
				)}
				value={value}
				onChange={(event) => {
					const target = event.currentTarget;
					target.style.height = "auto";
					target.style.height = `${target.scrollHeight}px`;

					onChange(event.target.value);
				}}
				disabled={disabled}
				required={required}
				placeholder={placeholder}
			/>
			{masked && (
				<Button
					type="button"
					variant="subtle"
					size="icon"
					onMouseDown={() => setShowMaskedInput(true)}
					onMouseOut={() => setShowMaskedInput(false)}
					onMouseUp={() => setShowMaskedInput(false)}
					disabled={disabled}
				>
					{showMaskedInput ? (
						<EyeOff className="h-4 w-4" />
					) : (
						<Eye className="h-4 w-4" />
					)}
				</Button>
			)}
		</Stack>
	);
};

type ParsedValues = {
	values: string[];
	error: string;
};

const parseStringArrayValue = (value: string): ParsedValues => {
	const parsedValues: ParsedValues = {
		values: [],
		error: "",
	};

	if (value) {
		try {
			const parsed = JSON.parse(value);
			if (Array.isArray(parsed)) {
				parsedValues.values = parsed;
			}
		} catch (e) {
			parsedValues.error = `Error parsing parameter of type list(string), ${e}`;
		}
	}

	return parsedValues;
};

interface OptionDisplayProps {
	option: PreviewParameterOption;
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
						<TooltipContent side="right" sideOffset={10}>
							{option.description}
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			)}
		</div>
	);
};

interface ParameterDiagnosticsProps {
	diagnostics: PreviewParameter["diagnostics"];
}

const ParameterDiagnostics: FC<ParameterDiagnosticsProps> = ({
	diagnostics,
}) => {
	return (
		<>
			{diagnostics.map((diagnostic, index) => {
				if (diagnostic.extra?.code === "required") {
					return null;
				}
				return (
					<div
						key={`parameter-diagnostic-${diagnostic.summary}-${index}`}
						className={`text-xs px-1 ${
							diagnostic.severity === "error"
								? "text-content-destructive"
								: "text-content-warning"
						}`}
					>
						<p className="font-medium">{diagnostic.summary}</p>
						{diagnostic.detail && <p className="m-0">{diagnostic.detail}</p>}
					</div>
				);
			})}
		</>
	);
};

export const getInitialParameterValues = (
	params: PreviewParameter[],
	autofillParams?: AutofillBuildParameter[],
): WorkspaceBuildParameter[] => {
	return params.map((parameter) => {
		// Short-circuit for ephemeral parameters, which are always reset to
		// the template-defined default.
		if (parameter.ephemeral) {
			return {
				name: parameter.name,
				value: validValue(parameter.value),
			};
		}

		const autofillParam = autofillParams?.find(
			({ name }) => name === parameter.name,
		);

		const useAutofill =
			autofillParam?.value && isValidParameterOption(parameter, autofillParam);

		return {
			name: parameter.name,
			value: useAutofill ? autofillParam.value : validValue(parameter.value),
		};
	});
};

const validValue = (value: NullHCLString) => {
	return value.valid ? value.value : "";
};

const isValidParameterOption = (
	previewParam: PreviewParameter,
	buildParam: WorkspaceBuildParameter,
) => {
	// multi-select is the only list(string) type with options
	if (previewParam.form_type === "multi-select") {
		let values: string[] = [];
		try {
			const parsed = JSON.parse(buildParam.value);
			if (Array.isArray(parsed)) {
				values = parsed;
			}
		} catch {
			return false;
		}

		if (previewParam.options.length > 0) {
			const validValues = previewParam.options.map(
				(option) => option.value.value,
			);
			return values.some((value) => validValues.includes(value));
		}
		return false;
	}

	// For parameters with options (dropdown, radio)
	if (previewParam.options.length > 0) {
		const validValues = previewParam.options.map(
			(option) => option.value.value,
		);
		return validValues.includes(buildParam.value);
	}

	// For parameters without options (input,textarea,switch,checkbox,tag-select)
	return true;
};

export const useValidationSchemaForDynamicParameters = (
	parameters?: PreviewParameter[],
	lastBuildParameters?: WorkspaceBuildParameter[],
): Yup.AnySchema => {
	if (!parameters) {
		return Yup.object();
	}

	return Yup.array()
		.of(
			Yup.object().shape({
				name: Yup.string().required(),
				value: Yup.string()
					.test("verify with template", (val, ctx) => {
						const name = ctx.parent.name;
						const parameter = parameters.find(
							(parameter) => parameter.name === name,
						);
						if (parameter) {
							switch (parameter.type) {
								case "number": {
									const minValidation = parameter.validations.find(
										(v) => v.validation_min !== null,
									);
									const maxValidation = parameter.validations.find(
										(v) => v.validation_max !== null,
									);

									if (
										minValidation &&
										minValidation.validation_min !== null &&
										!maxValidation &&
										Number(val) < minValidation.validation_min
									) {
										return ctx.createError({
											path: ctx.path,
											message:
												parameterError(parameter, val) ??
												`Value must be greater than ${minValidation.validation_min}.`,
										});
									}

									if (
										!minValidation &&
										maxValidation &&
										maxValidation.validation_max !== null &&
										Number(val) > maxValidation.validation_max
									) {
										return ctx.createError({
											path: ctx.path,
											message:
												parameterError(parameter, val) ??
												`Value must be less than ${maxValidation.validation_max}.`,
										});
									}

									if (
										minValidation &&
										minValidation.validation_min !== null &&
										maxValidation &&
										maxValidation.validation_max !== null &&
										(Number(val) < minValidation.validation_min ||
											Number(val) > maxValidation.validation_max)
									) {
										return ctx.createError({
											path: ctx.path,
											message:
												parameterError(parameter, val) ??
												`Value must be between ${minValidation.validation_min} and ${maxValidation.validation_max}.`,
										});
									}

									const monotonic = parameter.validations.find(
										(v) =>
											v.validation_monotonic !== null &&
											v.validation_monotonic !== "",
									);

									if (monotonic && lastBuildParameters) {
										const lastBuildParameter = lastBuildParameters.find(
											(last: { name: string }) => last.name === name,
										);
										if (lastBuildParameter) {
											switch (monotonic.validation_monotonic) {
												case "increasing":
													if (Number(lastBuildParameter.value) > Number(val)) {
														return ctx.createError({
															path: ctx.path,
															message: `Value must only ever increase (last value was ${lastBuildParameter.value})`,
														});
													}
													break;
												case "decreasing":
													if (Number(lastBuildParameter.value) < Number(val)) {
														return ctx.createError({
															path: ctx.path,
															message: `Value must only ever decrease (last value was ${lastBuildParameter.value})`,
														});
													}
													break;
											}
										}
									}
									break;
								}
								case "string": {
									const regex = parameter.validations.find(
										(v) =>
											v.validation_regex !== null && v.validation_regex !== "",
									);
									if (!regex || !regex.validation_regex) {
										return true;
									}

									if (val && !new RegExp(regex.validation_regex).test(val)) {
										return ctx.createError({
											path: ctx.path,
											message: parameterError(parameter, val),
										});
									}
									break;
								}
							}
						}
						return true;
					}),
			}),
		)
		.required();
};

const parameterError = (
	parameter: PreviewParameter,
	value?: string,
): string | undefined => {
	const validation_error = parameter.validations.find(
		(v) => v.validation_error !== null,
	);
	const minValidation = parameter.validations.find(
		(v) => v.validation_min !== null,
	);
	const maxValidation = parameter.validations.find(
		(v) => v.validation_max !== null,
	);

	if (!validation_error || !value) {
		return;
	}

	const r = new Map<string, string>([
		[
			"{min}",
			minValidation ? (minValidation.validation_min?.toString() ?? "") : "",
		],
		[
			"{max}",
			maxValidation ? (maxValidation.validation_max?.toString() ?? "") : "",
		],
		["{value}", value],
	]);
	return validation_error.validation_error.replace(
		/{min}|{max}|{value}/g,
		(match) => r.get(match) || "",
	);
};

interface DiagnosticsProps {
	diagnostics: PreviewParameter["diagnostics"];
}

export const Diagnostics: FC<DiagnosticsProps> = ({ diagnostics }) => {
	return (
		<div className="flex flex-col gap-4">
			{diagnostics.map((diagnostic, index) => (
				<div
					key={`diagnostic-${diagnostic.summary}-${index}`}
					className={cn(
						"text-xs font-semibold flex flex-col rounded-md border px-3.5 py-3.5 border-solid",
						diagnostic.severity === "error"
							? "text-content-primary border-border-destructive bg-content-destructive/15"
							: "text-content-primary border-border-warning bg-content-warning/15",
					)}
				>
					<div className="flex flex-row items-start">
						{diagnostic.severity === "error" && (
							<CircleAlert
								className="me-2 inline-flex shrink-0 text-content-destructive size-icon-sm"
								aria-hidden="true"
							/>
						)}
						{diagnostic.severity === "warning" && (
							<TriangleAlert
								className="me-2 inline-flex shrink-0 text-content-warning size-icon-sm"
								aria-hidden="true"
							/>
						)}
						<div className="flex flex-col gap-3">
							<p className="m-0">{diagnostic.summary}</p>
							{diagnostic.detail && <p className="m-0">{diagnostic.detail}</p>}
						</div>
					</div>
				</div>
			))}
		</div>
	);
};
