import type { ComponentPropsWithRef, FC, ReactNode } from "react";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { cn } from "#/utils/cn";
import { usePasswordValidator } from "./usePasswordValidator";

type PasswordFieldProps = Omit<ComponentPropsWithRef<"input">, "value"> & {
	label: string;
	value: string;
	error?: boolean;
	helperText?: ReactNode;
};

/**
 * A password field component that validates the password against the API with
 * debounced calls. It uses a debounced value to minimize the number of API
 * calls and displays validation errors.
 */
export const PasswordField: FC<PasswordFieldProps> = ({
	label,
	error,
	helperText,
	id,
	value,
	...props
}) => {
	const { valid, details } = usePasswordValidator(value);
	const isInvalid = !valid || error;
	const displayHelper = !valid ? details : helperText;

	return (
		<div className="flex flex-col items-start gap-1">
			<Label htmlFor={id}>{label}</Label>
			<Input
				{...props}
				id={id}
				type="password"
				value={value}
				aria-invalid={isInvalid || undefined}
			/>
			{displayHelper && (
				<span
					className={cn(
						"text-xs text-left",
						isInvalid ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{displayHelper}
				</span>
			)}
		</div>
	);
};
