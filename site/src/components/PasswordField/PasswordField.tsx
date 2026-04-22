import type { ComponentProps, FC } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { API } from "#/api/api";
import { FormField } from "#/components/FormField/FormField";
import { useDebouncedValue } from "#/hooks/debounce";

type PasswordFieldProps = ComponentProps<typeof FormField>;

/**
 * A password field component that validates the password against the API with
 * debounced calls. It uses a debounced value to minimize the number of API
 * calls and displays validation errors.
 */
export const PasswordField: FC<PasswordFieldProps> = ({ field, ...props }) => {
	const value = field.value === undefined ? "" : String(field.value);
	const debouncedValue = useDebouncedValue(value, 500);
	const validatePasswordQuery = useQuery({
		queryKey: ["validatePassword", debouncedValue],
		queryFn: () => API.validateUserPassword(debouncedValue),
		placeholderData: keepPreviousData,
		enabled: debouncedValue.length > 0,
	});
	const invalidPassword = validatePasswordQuery.data?.valid === false;
	const helperText = invalidPassword
		? (validatePasswordQuery.data?.details ?? field.helperText)
		: field.helperText;
	const mergedField = {
		...field,
		error: field.error || invalidPassword,
		helperText,
	};

	return (
		<FormField {...props} type={props.type ?? "password"} field={mergedField} />
	);
};
