import TextField, { type TextFieldProps } from "@mui/material/TextField";
import { API } from "api/api";
import { useDebouncedValue } from "hooks/debounce";
import type { FC } from "react";
import { useQuery } from "react-query";

// TODO: @BrunoQuaresma: Unable to integrate Yup + Formik for validation. The
// validation was triggering on the onChange event, but the form.errors were not
// updating accordingly. Tried various combinations of validateOnBlur and
// validateOnChange without success. Further investigation is needed.

/**
 * A password field component that validates the password against the API with
 * debounced calls. It uses a debounced value to minimize the number of API
 * calls and displays validation errors.
 */
export const PasswordField: FC<TextFieldProps> = (props) => {
	const debouncedValue = useDebouncedValue(`${props.value}`, 500);
	const validatePasswordQuery = useQuery({
		queryKey: ["validatePassword", debouncedValue],
		queryFn: () => API.validateUserPassword(debouncedValue),
		keepPreviousData: true,
		enabled: debouncedValue.length > 0,
	});
	const valid = validatePasswordQuery.data?.valid ?? true;

	return (
		<TextField
			{...props}
			type="password"
			error={!valid || props.error}
			helperText={
				!valid ? validatePasswordQuery.data?.details : props.helperText
			}
		/>
	);
};
