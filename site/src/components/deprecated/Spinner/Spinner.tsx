import CircularProgress, {
	type CircularProgressProps,
} from "@mui/material/CircularProgress";
import isChromatic from "chromatic/isChromatic";
import type { FC } from "react";

/**
 * Spinner component used to indicate loading states. This component abstracts
 * the MUI CircularProgress to provide better control over its rendering,
 * especially in snapshot tests with Chromatic.
 *
 * @deprecated prefer `components.Spinner`
 */
export const Spinner: FC<CircularProgressProps> = (props) => {
	/**
	 * During Chromatic snapshots, we render the spinner as determinate to make it
	 * static without animations, using a deterministic value (75%).
	 */
	if (isChromatic()) {
		props.variant = "determinate";
		props.value = 75;
	}
	return <CircularProgress {...props} />;
};
