import type { ComponentPropsWithRef, FC, ReactNode } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { API } from "#/api/api";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { useDebouncedValue } from "#/hooks/debounce";
import { cn } from "#/utils/cn";

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
	const debouncedValue = useDebouncedValue(value, 500);
	const { data } = useQuery({
		queryKey: ["validatePassword", debouncedValue],
		queryFn: () => API.validateUserPassword(debouncedValue),
		placeholderData: keepPreviousData,
		enabled: debouncedValue.length > 0,
	});

	const valid = data?.valid ?? true;
	const isInvalid = !valid || error;
	const displayHelper = !valid ? data?.details : helperText;

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
