import type { FC } from "react";
import { Input, type InputProps } from "#/components/Input/Input";
import { cn } from "#/utils/cn";

/**
 * Sentinel value representing an existing secret that the backend
 * will not reveal. Displayed as masked dots in the input field.
 */
export const SECRET_PLACEHOLDER = "••••••••••••••••";

/**
 * Extracts the real secret value from a form field that uses the
 * placeholder pattern. Returns the trimmed value when the field
 * has been touched and differs from the placeholder, or undefined
 * when the user has not modified the field.
 */
export function effectiveSecretValue(
	value: string,
	touched: boolean,
): string | undefined {
	if (!touched || value === SECRET_PLACEHOLDER) {
		return undefined;
	}
	return value.trim() || undefined;
}

interface SecretInputProps
	extends Omit<InputProps, "type" | "onChange" | "onFocus" | "value"> {
	value: string;
	touched: boolean;
	onValueChange: (value: string, touched: boolean) => void;
}

/**
 * Controlled input for secret/API-key fields that masks an
 * existing value with a placeholder and clears it on first focus.
 *
 * Uses `type="text"` with CSS disc-masking rather than
 * `type="password"` to avoid browser autofill and save-password
 * prompts while still hiding the value visually.
 */
export const SecretInput: FC<SecretInputProps> = ({
	value,
	touched,
	onValueChange,
	className,
	...rest
}) => (
	<Input
		type="text"
		autoComplete="off"
		data-1p-ignore
		data-lpignore="true"
		data-form-type="other"
		data-bwignore
		className={cn(
			"h-9 font-mono text-[13px] [-webkit-text-security:disc]",
			className,
		)}
		value={value}
		onFocus={() => {
			if (!touched && value === SECRET_PLACEHOLDER) {
				onValueChange("", true);
			}
		}}
		onChange={(e) => onValueChange(e.target.value, true)}
		{...rest}
	/>
);
