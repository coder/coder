import type { Interpolation, Theme } from "@emotion/react";
import { CircleAlertIcon, SettingsIcon } from "lucide-react";
import { type FC, type ReactNode, useId, useState } from "react";
import type { TemplateVersionParameter } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { Input, type InputProps } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { MemoizedMarkdown } from "#/components/Markdown/Markdown";
import { Pill } from "#/components/Pill/Pill";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import { Stack } from "#/components/Stack/Stack";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import type {
	AutofillBuildParameter,
	AutofillSource,
} from "#/utils/richParameters";
import { TagInput } from "../TagInput/TagInput";

const isBoolean = (parameter: TemplateVersionParameter) => {
	return parameter.type === "bool";
};

const getParameterDisplayName = (parameter: TemplateVersionParameter) => {
	return parameter.display_name ? parameter.display_name : parameter.name;
};

const styles = {
	labelCaption: (theme) => ({
		fontSize: 14,
		color: theme.palette.text.secondary,

		".small &": {
			fontSize: 13,
			lineHeight: "140%",
		},
	}),
	labelPrimary: (theme) => ({
		fontSize: 16,
		color: theme.palette.text.primary,
		fontWeight: 500,
		display: "flex",
		alignItems: "center",
		flexWrap: "wrap",
		gap: 8,

		"& p": {
			margin: 0,
			lineHeight: "24px", // Keep the same as ParameterInput
		},

		".small &": {
			fontSize: 14,
		},
	}),
	optionalLabel: (theme) => ({
		fontSize: 14,
		color: theme.palette.text.disabled,
		fontWeight: 500,
	}),
	labelIconWrapper: {
		width: 20,
		height: 20,
		display: "block",
		flexShrink: 0,

		".small &": {
			display: "none",
		},
	},
	labelIcon: {
		width: "100%",
		height: "100%",
		objectFit: "contain",
	},
	optionIcon: {
		pointerEvents: "none",
		maxHeight: 20,
		width: 20,

		".small &": {
			maxHeight: 16,
			width: 16,
		},
	},
} satisfies Record<string, Interpolation<Theme>>;

interface ParameterLabelProps {
	parameter: TemplateVersionParameter;
	isPreset?: boolean;
	id?: string;
	htmlFor?: string;
}

const ParameterLabel: FC<ParameterLabelProps> = ({
	parameter,
	isPreset,
	id,
	htmlFor,
}) => {
	const hasDescription = parameter.description && parameter.description !== "";
	const displayName = getParameterDisplayName(parameter);

	const labelPrimary = (
		<span css={styles.labelPrimary}>
			{displayName}

			{!parameter.required && (
				<Tooltip>
					<TooltipTrigger asChild>
						<span css={styles.optionalLabel}>(optional)</span>
					</TooltipTrigger>
					<TooltipContent side="bottom" className="max-w-xs">
						If no value is specified, the system will default to the value set
						by the administrator.
					</TooltipContent>
				</Tooltip>
			)}
			{!parameter.mutable && (
				<Tooltip>
					<TooltipTrigger asChild>
						<Pill
							type="warning"
							icon={<CircleAlertIcon className="size-icon-xs" />}
						>
							Immutable
						</Pill>
					</TooltipTrigger>
					<TooltipContent side="bottom" className="max-w-xs">
						This value cannot be modified after the workspace has been created.
					</TooltipContent>
				</Tooltip>
			)}
			{isPreset && (
				<Tooltip>
					<TooltipTrigger asChild>
						<Pill type="info" icon={<SettingsIcon className="size-icon-xs" />}>
							Preset
						</Pill>
					</TooltipTrigger>
					<TooltipContent side="bottom">
						This value was set by a preset
					</TooltipContent>
				</Tooltip>
			)}
		</span>
	);

	const primaryLabel = htmlFor ? (
		<Label htmlFor={htmlFor} className="block cursor-default p-0">
			{labelPrimary}
		</Label>
	) : (
		labelPrimary
	);

	return (
		<div id={id}>
			<Stack direction="row" alignItems="center">
				{parameter.icon && (
					<span css={styles.labelIconWrapper}>
						<ExternalImage
							css={styles.labelIcon}
							alt="Parameter icon"
							src={parameter.icon}
						/>
					</span>
				)}

				{hasDescription ? (
					<Stack spacing={0}>
						{primaryLabel}
						<MemoizedMarkdown css={styles.labelCaption}>
							{parameter.description}
						</MemoizedMarkdown>
					</Stack>
				) : (
					primaryLabel
				)}
			</Stack>
		</div>
	);
};

type Size = "medium" | "small";

type RichParameterInputProps = Omit<
	InputProps,
	"size" | "onChange" | "value"
> & {
	parameter: TemplateVersionParameter;
	parameterAutofill?: AutofillBuildParameter;
	onChange: (value: string) => void;
	size?: Size;
	isPreset?: boolean;
	value?: string | number;
	error?: boolean;
	helperText?: ReactNode;
	inputProps?: InputProps;
	label?: ReactNode;
};

const autofillDescription: Partial<Record<AutofillSource, ReactNode>> = {
	url: " from the URL.",
};

export const RichParameterInput: FC<RichParameterInputProps> = ({
	size = "medium",
	parameter,
	parameterAutofill,
	onChange,
	isPreset,
	helperText,
	error,
	id,
	"aria-describedby": ariaDescribedBy,
	...fieldProps
}) => {
	const generatedId = useId();
	const inputId = id ?? parameter.name ?? generatedId;
	const labelId = `${inputId}-label`;
	const helperTextId = helperText ? `${inputId}-helper-text` : undefined;
	const autofillSource = parameterAutofill?.source;
	const autofillValue = parameterAutofill?.value;
	const [hideSuggestion, setHideSuggestion] = useState(false);
	const showUserHistorySuggestion = Boolean(
		!parameter.ephemeral &&
			autofillSource === "user_history" &&
			autofillValue &&
			!hideSuggestion,
	);
	const suggestionId = showUserHistorySuggestion
		? `${inputId}-autofill-suggestion`
		: undefined;
	const autofillTextId =
		autofillSource && autofillDescription[autofillSource]
			? `${inputId}-autofill-helper`
			: undefined;
	const describedBy = [
		ariaDescribedBy,
		helperTextId,
		suggestionId,
		autofillTextId,
	]
		.filter(Boolean)
		.join(" ");
	const isGroup = isBoolean(parameter) || parameter.options.length > 0;

	return (
		<Stack
			direction="column"
			spacing={size === "small" ? 1.25 : 2}
			className={size}
			data-testid={`parameter-field-${parameter.name}`}
		>
			<ParameterLabel
				id={labelId}
				htmlFor={isGroup ? undefined : inputId}
				parameter={parameter}
				isPreset={isPreset}
			/>
			<div className="flex flex-col">
				<RichParameterField
					{...fieldProps}
					describedBy={describedBy || undefined}
					error={error}
					inputId={inputId}
					labelId={labelId}
					onChange={onChange}
					size={size}
					parameter={parameter}
					parameterAutofill={parameterAutofill}
				/>
				{helperText && (
					<p
						id={helperTextId}
						className={cn(
							"m-0 mt-1 text-xs",
							error ? "text-content-destructive" : "text-content-secondary",
						)}
					>
						{helperText}
					</p>
				)}
				{showUserHistorySuggestion && (
					<p
						id={suggestionId}
						className="m-0 mt-1 text-xs text-content-secondary"
					>
						<Button
							variant="subtle"
							size="xs"
							className="-ml-1 h-auto min-w-0 px-1.5 py-1 text-xs text-content-warning hover:text-content-primary"
							onClick={() => {
								onChange(autofillValue ?? "");
								setHideSuggestion(true);
							}}
						>
							{autofillValue}
						</Button>{" "}
						was recently used for this parameter.
					</p>
				)}
				{autofillSource && autofillDescription[autofillSource] && (
					<p
						id={autofillTextId}
						className="m-0 mt-1 text-xs text-content-secondary"
					>
						🪄 Autofilled {autofillDescription[autofillSource]}
					</p>
				)}
			</div>
		</Stack>
	);
};

type RichParameterFieldProps = RichParameterInputProps & {
	inputId: string;
	labelId: string;
	describedBy?: string;
};

const RichParameterField: FC<RichParameterFieldProps> = ({
	className,
	describedBy,
	disabled,
	error,
	inputId,
	inputProps,
	label,
	labelId,
	onChange,
	parameter,
	parameterAutofill,
	value,
	size,
	helperText: _helperText,
	isPreset: _isPreset,
	...props
}) => {
	const small = size === "small";
	const radioValue = value === undefined ? "" : String(value);

	if (isBoolean(parameter)) {
		return (
			<RadioGroup
				id={inputId}
				data-testid="parameter-field-bool"
				value={radioValue}
				onValueChange={onChange}
				disabled={disabled}
				name={props.name}
				aria-labelledby={labelId}
				aria-describedby={describedBy}
				aria-invalid={error || undefined}
				className={cn(small ? "gap-1.5" : "gap-2")}
			>
				<RadioOption
					id={`${inputId}-true`}
					label="True"
					value="true"
					disabled={disabled}
					error={error}
					small={small}
				/>
				<RadioOption
					id={`${inputId}-false`}
					label="False"
					value="false"
					disabled={disabled}
					error={error}
					small={small}
				/>
			</RadioGroup>
		);
	}

	if (parameter.options.length > 0) {
		return (
			<RadioGroup
				id={inputId}
				data-testid="parameter-field-options"
				value={radioValue}
				onValueChange={onChange}
				disabled={disabled}
				name={props.name}
				aria-labelledby={labelId}
				aria-describedby={describedBy}
				aria-invalid={error || undefined}
				className={cn(small ? "gap-1.5" : "gap-2")}
			>
				{parameter.options.map((option, index) => (
					<RadioOption
						key={option.name}
						id={`${inputId}-option-${index}`}
						label={
							<Stack direction="row" alignItems="center">
								{option.icon && (
									<ExternalImage
										css={styles.optionIcon}
										src={option.icon}
										alt="Parameter icon"
									/>
								)}
								{option.description ? (
									<Stack
										spacing={small ? 1 : 0}
										alignItems={small ? "center" : undefined}
										direction={small ? "row" : "column"}
										className={small ? undefined : "py-1"}
									>
										{small ? (
											<Tooltip>
												<TooltipTrigger asChild>
													<span>{option.name}</span>
												</TooltipTrigger>
												<TooltipContent side="bottom" className="max-w-xs">
													<MemoizedMarkdown>
														{option.description}
													</MemoizedMarkdown>
												</TooltipContent>
											</Tooltip>
										) : (
											<>
												<span>{option.name}</span>
												<MemoizedMarkdown css={styles.labelCaption}>
													{option.description}
												</MemoizedMarkdown>
											</>
										)}
									</Stack>
								) : (
									option.name
								)}
							</Stack>
						}
						value={option.value}
						disabled={disabled}
						error={error}
						small={small}
					/>
				))}
			</RadioGroup>
		);
	}

	if (parameter.type === "list(string)") {
		let values: string[] = [];

		if (typeof value !== "string") {
			throw new Error("Expected value to be a string");
		}

		if (value) {
			try {
				values = JSON.parse(value) as string[];
			} catch (e) {
				console.error("Error parsing list(string) parameter", e);
			}
		}

		return (
			<TagInput
				id={inputId}
				data-testid="parameter-field-list-of-string"
				label={
					typeof label === "string" ? label : getParameterDisplayName(parameter)
				}
				values={values}
				onChange={(values) => {
					try {
						const value = JSON.stringify(values);
						onChange(value);
					} catch (e) {
						console.error("Error on change of list(string) parameter", e);
					}
				}}
			/>
		);
	}

	let numberInputProps: InputProps = {};
	if (parameter.type === "number") {
		switch (parameter.validation_monotonic) {
			case "increasing":
				numberInputProps = {
					max: parameter.validation_max,
					min: parameterAutofill?.value,
				};
				break;
			case "decreasing":
				numberInputProps = {
					max: parameterAutofill?.value,
					min: parameter.validation_min,
				};
				break;
			default:
				numberInputProps = {
					max: parameter.validation_max,
					min: parameter.validation_min,
				};
				break;
		}
	}

	// A text field can technically handle all cases!
	// As other cases become more prominent (like filtering for numbers),
	// we should break this out into more finely scoped input fields.
	return (
		<Input
			{...inputProps}
			{...props}
			{...numberInputProps}
			id={inputId}
			data-testid="parameter-field-text"
			type={parameter.type}
			disabled={disabled}
			required={parameter.required}
			placeholder={parameter.default_value}
			value={value}
			aria-invalid={error || undefined}
			aria-describedby={describedBy}
			className={cn(
				small && "h-9 rounded-md text-sm",
				inputProps?.className,
				className,
			)}
			onChange={(event) => {
				onChange(event.target.value);
			}}
		/>
	);
};

type RadioOptionProps = {
	id: string;
	label: ReactNode;
	value: string;
	disabled?: boolean;
	error?: boolean;
	small?: boolean;
};

const RadioOption: FC<RadioOptionProps> = ({
	id,
	label,
	value,
	disabled,
	error,
	small,
}) => {
	return (
		<div className="flex items-center gap-2">
			<RadioGroupItem
				id={id}
				value={value}
				disabled={disabled}
				aria-invalid={error || undefined}
				className={cn(error && "border-border-destructive")}
			/>
			<Label
				htmlFor={id}
				className={cn(
					"min-w-0 cursor-pointer font-normal leading-normal text-content-primary",
					small ? "text-sm" : "text-base",
					disabled && "cursor-not-allowed opacity-50",
				)}
			>
				{label}
			</Label>
		</div>
	);
};
