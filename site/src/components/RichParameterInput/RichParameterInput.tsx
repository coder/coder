import type { Interpolation, Theme } from "@emotion/react";
import ErrorOutline from "@mui/icons-material/ErrorOutline";
import SettingsIcon from "@mui/icons-material/Settings";
import Button from "@mui/material/Button";
import FormControlLabel from "@mui/material/FormControlLabel";
import FormHelperText from "@mui/material/FormHelperText";
import type { InputBaseComponentProps } from "@mui/material/InputBase";
import Radio from "@mui/material/Radio";
import RadioGroup from "@mui/material/RadioGroup";
import TextField, { type TextFieldProps } from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import type { TemplateVersionParameter } from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { MemoizedMarkdown } from "components/Markdown/Markdown";
import { Pill } from "components/Pill/Pill";
import { Stack } from "components/Stack/Stack";
import { type FC, type ReactNode, useState } from "react";
import type {
	AutofillBuildParameter,
	AutofillSource,
} from "utils/richParameters";
import { MultiTextField } from "./MultiTextField";

const isBoolean = (parameter: TemplateVersionParameter) => {
	return parameter.type === "bool";
};

const styles = {
	label: {
		marginBottom: 4,
	},
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
	textField: {
		".small & .MuiInputBase-root": {
			height: 36,
			fontSize: 14,
			borderRadius: 6,
		},
	},
	radioGroup: {
		".small & .MuiFormControlLabel-label": {
			fontSize: 14,
		},
		".small & .MuiRadio-root": {
			padding: "6px 9px", // 8px + 1px border
		},
		".small & .MuiRadio-root svg": {
			width: 16,
			height: 16,
		},
	},
	checkbox: {
		display: "flex",
		alignItems: "center",
		gap: 8,
	},
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
	suggestion: (theme) => ({
		color: theme.roles.notice.fill.solid,
		marginLeft: "-4px",
		padding: "4px 6px",
		lineHeight: "inherit",
		fontSize: "inherit",
		height: "unset",
		minWidth: "unset",
	}),
} satisfies Record<string, Interpolation<Theme>>;

export interface ParameterLabelProps {
	parameter: TemplateVersionParameter;
	isPreset?: boolean;
}

const ParameterLabel: FC<ParameterLabelProps> = ({ parameter, isPreset }) => {
	const hasDescription = parameter.description && parameter.description !== "";
	const displayName = parameter.display_name
		? parameter.display_name
		: parameter.name;

	const labelPrimary = (
		<span css={styles.labelPrimary}>
			{displayName}

			{!parameter.required && (
				<Tooltip title="If no value is specified, the system will default to the value set by the administrator.">
					<span css={styles.optionalLabel}>(optional)</span>
				</Tooltip>
			)}
			{!parameter.mutable && (
				<Tooltip title="This value cannot be modified after the workspace has been created.">
					<Pill type="warning" icon={<ErrorOutline />}>
						Immutable
					</Pill>
				</Tooltip>
			)}
			{isPreset && (
				<Tooltip title="This value was set by a preset">
					<Pill type="info" icon={<SettingsIcon />}>
						Preset
					</Pill>
				</Tooltip>
			)}
		</span>
	);

	return (
		<label htmlFor={parameter.name}>
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
						{labelPrimary}
						<MemoizedMarkdown css={styles.labelCaption}>
							{parameter.description}
						</MemoizedMarkdown>
					</Stack>
				) : (
					labelPrimary
				)}
			</Stack>
		</label>
	);
};

type Size = "medium" | "small";

export type RichParameterInputProps = Omit<
	TextFieldProps,
	"size" | "onChange"
> & {
	parameter: TemplateVersionParameter;
	parameterAutofill?: AutofillBuildParameter;
	onChange: (value: string) => void;
	size?: Size;
	isPreset?: boolean;
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
	...fieldProps
}) => {
	const autofillSource = parameterAutofill?.source;
	const autofillValue = parameterAutofill?.value;
	const [hideSuggestion, setHideSuggestion] = useState(false);

	return (
		<Stack
			direction="column"
			spacing={size === "small" ? 1.25 : 2}
			className={size}
			data-testid={`parameter-field-${parameter.name}`}
		>
			<ParameterLabel parameter={parameter} isPreset={isPreset} />
			<div css={{ display: "flex", flexDirection: "column" }}>
				<RichParameterField
					{...fieldProps}
					onChange={onChange}
					size={size}
					parameter={parameter}
					parameterAutofill={parameterAutofill}
				/>
				{!parameter.ephemeral &&
					autofillSource === "user_history" &&
					autofillValue &&
					!hideSuggestion && (
						<FormHelperText>
							<Button
								variant="text"
								css={styles.suggestion}
								onClick={() => {
									onChange(autofillValue);
									setHideSuggestion(true);
								}}
							>
								{autofillValue}
							</Button>{" "}
							was recently used for this parameter.
						</FormHelperText>
					)}
				{autofillSource && autofillDescription[autofillSource] && (
					<div css={{ marginTop: 4, fontSize: 12 }}>
						🪄 Autofilled {autofillDescription[autofillSource]}
					</div>
				)}
			</div>
		</Stack>
	);
};

const RichParameterField: FC<RichParameterInputProps> = ({
	disabled,
	onChange,
	parameter,
	parameterAutofill,
	value,
	size,
	...props
}) => {
	const small = size === "small";

	if (isBoolean(parameter)) {
		return (
			<RadioGroup
				id={parameter.name}
				data-testid="parameter-field-bool"
				css={styles.radioGroup}
				value={value}
				onChange={(_, value) => onChange(value)}
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

	if (parameter.options.length > 0) {
		return (
			<RadioGroup
				id={parameter.name}
				data-testid="parameter-field-options"
				css={styles.radioGroup}
				value={value}
				onChange={(_, value) => onChange(value)}
			>
				{parameter.options.map((option) => (
					<FormControlLabel
						disabled={disabled}
						key={option.name}
						value={option.value}
						control={<Radio size="small" />}
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
										css={{ padding: small ? undefined : "4px 0" }}
									>
										{small ? (
											<Tooltip
												title={
													<MemoizedMarkdown>
														{option.description}
													</MemoizedMarkdown>
												}
											>
												<div>{option.name}</div>
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
			<MultiTextField
				id={parameter.name}
				data-testid="parameter-field-list-of-string"
				label={props.label as string}
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

	let inputProps: InputBaseComponentProps = {};
	if (parameter.type === "number") {
		switch (parameter.validation_monotonic) {
			case "increasing":
				inputProps = {
					max: parameter.validation_max,
					min: parameterAutofill?.value,
				};
				break;
			case "decreasing":
				inputProps = {
					max: parameterAutofill?.value,
					min: parameter.validation_min,
				};
				break;
			default:
				inputProps = {
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
		<TextField
			{...props}
			id={parameter.name}
			data-testid="parameter-field-text"
			css={styles.textField}
			type={parameter.type}
			disabled={disabled}
			required={parameter.required}
			placeholder={parameter.default_value}
			value={value}
			inputProps={inputProps}
			onChange={(event) => {
				onChange(event.target.value);
			}}
		/>
	);
};
