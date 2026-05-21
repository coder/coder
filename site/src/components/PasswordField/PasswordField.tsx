import type { FC } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { API } from "#/api/api";
import { Input, type InputProps } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { useDebouncedValue } from "#/hooks/debounce";
import { cn } from "#/utils/cn";
import type { FormHelpers } from "#/utils/formUtils";

type PasswordFieldProps = InputProps & {
	label: string;
	field: FormHelpers;
};

/**
 * A password field component that validates the password against the API with
 * debounced calls. It uses a debounced value to minimize the number of API
 * calls and displays validation errors.
 */
export const PasswordField: FC<PasswordFieldProps> = ({
	label,
	field,
	...props
}) => {
	const value = field.value === undefined ? "" : String(field.value);
	const debouncedValue = useDebouncedValue(value, 500);
	const validatePasswordQuery = useQuery({
		queryKey: ["validatePassword", debouncedValue],
		queryFn: () => API.validateUserPassword(debouncedValue),
		placeholderData: keepPreviousData,
		enabled: debouncedValue.length > 0,
	});
	const valid = validatePasswordQuery.data?.valid ?? true;

	const displayHelper = !valid
		? validatePasswordQuery.data?.details
		: field.helperText;

	return (
		<div className="flex flex-col items-start gap-2">
			<Label htmlFor={field.id}>{label}</Label>
			<Input
				id={field.id}
				type="password"
				name={field.name}
				value={field.value}
				onChange={field.onChange}
				onBlur={field.onBlur}
				{...props}
				aria-invalid={!valid || undefined}
			/>
			{displayHelper && (
				<span
					className={cn(
						"text-xs text-left",
						valid ? "text-content-secondary" : "text-content-destructive",
					)}
				>
					{displayHelper}
				</span>
			)}
		</div>
	);
};
