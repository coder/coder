import { useId } from "react";
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
	onBlur?: () => void;
	onFocus?: () => void;
};

export const CredentialField: React.FC<CredentialFieldProps> = ({
	label,
	helpers,
	placeholder,
	description,
	required = false,
	onBlur,
	onFocus,
}) => {
	const inputId = useId();
	const errorId = `${inputId}-error`;
	const helperId = `${inputId}-helper`;
	const descriptionId = `${inputId}-description`;
	const describedBy = [
		description ? descriptionId : null,
		helpers.error ? errorId : helpers.helperText ? helperId : null,
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
			// Credentials differ per provider, so browsers and password
			// managers must not refill a previously entered key.
			{...passwordManagerIgnoreProps}
			id={inputId}
			name={helpers.name}
			className="font-mono text-sm font-normal"
			value={helpers.value}
			onChange={helpers.onChange}
			onBlur={(event) => {
				helpers.onBlur(event);
				onBlur?.();
			}}
			onFocus={onFocus}
			placeholder={placeholder}
			aria-invalid={helpers.error}
			aria-describedby={describedBy || undefined}
		/>
	);

	return (
		<div className="flex flex-col gap-2">
			{labelNode}
			{descriptionNode}
			{inputNode}
			{helperNode}
		</div>
	);
};
