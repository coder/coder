import { useId, useState } from "react";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	type FormHelpers,
	passwordManagerIgnoreProps,
} from "#/utils/formUtils";

type CredentialFieldProps = {
	label: string;
	helpers: FormHelpers;
	placeholder?: string;
	description?: React.ReactNode;
	required?: boolean;
	/** Shown in place of an empty value while the field is not focused. */
	savedMask?: string;
};

export const CredentialField: React.FC<CredentialFieldProps> = ({
	label,
	helpers,
	placeholder,
	description,
	required = false,
	savedMask,
}) => {
	const inputId = useId();
	const [isFocused, setIsFocused] = useState(false);
	const errorId = `${inputId}-error`;
	const helperId = `${inputId}-helper`;
	const descriptionId = `${inputId}-description`;
	const describedBy = [
		helpers.error ? errorId : helpers.helperText ? helperId : null,
		description ? descriptionId : null,
	]
		.filter(Boolean)
		.join(" ");

	const labelNode = (
		<Label htmlFor={inputId}>
			{label}{" "}
			{required && (
				<span className="text-xs font-bold text-content-destructive">*</span>
			)}
		</Label>
	);

	const descriptionNode = description && (
		<div id={descriptionId} className="text-xs text-content-secondary">
			{description}
		</div>
	);

	const helperNode = helpers.error ? (
		<span id={errorId} className="text-xs text-content-destructive">
			{helpers.helperText}
		</span>
	) : helpers.helperText ? (
		<span id={helperId} className="text-xs text-content-secondary">
			{helpers.helperText}
		</span>
	) : null;

	const inputNode = (
		<Input
			{...passwordManagerIgnoreProps}
			id={inputId}
			name={helpers.name}
			className="font-mono text-sm font-normal"
			value={
				!isFocused && helpers.value === "" && savedMask
					? savedMask
					: helpers.value
			}
			onChange={helpers.onChange}
			onBlur={(event) => {
				helpers.onBlur(event);
				setIsFocused(false);
			}}
			onFocus={() => setIsFocused(true)}
			placeholder={placeholder}
			aria-invalid={helpers.error}
			aria-describedby={describedBy || undefined}
		/>
	);

	return (
		<div className="flex flex-col gap-2">
			{labelNode}
			{inputNode}
			{(helperNode || descriptionNode) && (
				<div className="flex flex-wrap items-center gap-x-2 gap-y-1">
					{helperNode}
					{descriptionNode}
				</div>
			)}
		</div>
	);
};
