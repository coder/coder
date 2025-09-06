import FormHelperText, {
	type FormHelperTextProps,
} from "@mui/material/FormHelperText";
import { Stack } from "components/Stack/Stack";
import type { ComponentProps, FC } from "react";

/**
 * Use these components as the label in FormControlLabel when implementing radio
 * buttons, checkboxes, or switches to ensure proper styling.
 */

export const StackLabel: FC<ComponentProps<typeof Stack>> = (props) => {
	return <Stack spacing={0.5} className="pl-3 font-medium" {...props} />;
};

export const StackLabelHelperText: FC<FormHelperTextProps> = (props) => {
	return (
		<FormHelperText
			className="mt-0 [&_strong]:text-content-primary"
			{...props}
		/>
	);
};
