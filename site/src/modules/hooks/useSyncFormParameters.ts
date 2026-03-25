import type * as TypesGen from "api/typesGenerated";
import type { PreviewParameter } from "api/typesGenerated";
import { type RefObject, useEffect, useRef } from "react";

type UseSyncFormParametersProps = {
	parameters: readonly PreviewParameter[];
	formValues: readonly TypesGen.WorkspaceBuildParameter[];
	setFieldValue: (
		field: string,
		value: TypesGen.WorkspaceBuildParameter[],
	) => void;
	// A ref holding the most recent parameter values sent to the
	// WebSocket. Used to detect stale responses: when the server
	// echoes back the same value we sent but the form has already
	// moved on (the user kept typing), we preserve the form value
	// instead of overwriting it.
	lastSentValues?: RefObject<Map<string, string>>;
};

export function useSyncFormParameters({
	parameters,
	formValues,
	setFieldValue,
	lastSentValues,
}: UseSyncFormParametersProps) {
	// Form values only needs to be updated when parameters change
	// Keep track of form values in a ref to avoid unnecessary updates to rich_parameter_values
	const formValuesRef = useRef(formValues);

	formValuesRef.current = formValues;

	// biome-ignore lint/correctness/useExhaustiveDependencies: lastSentValues is a stable ref whose .current is read lazily inside the effect.
	useEffect(() => {
		if (!parameters) return;
		const currentFormValues = formValuesRef.current;
		const currentFormValuesMap = new Map(
			currentFormValues.map((value) => [value.name, value.value]),
		);

		const newParameterValues = parameters.map((param) => {
			// When the server value is not valid (e.g., the initial
			// WebSocket response before any user input is sent),
			// preserve the current form value. This prevents the sync
			// hook from overwriting autofilled values (from the
			// previous build) with empty strings before the server
			// has had a chance to process them.
			if (!param.value.valid) {
				const existingValue = currentFormValuesMap.get(param.name);
				if (existingValue !== undefined) {
					return { name: param.name, value: existingValue };
				}
			}

			// Detect stale WebSocket responses. If the server
			// returned the exact value we last sent but the form
			// already holds something newer (the user kept typing
			// after the request was fired), preserve the form value
			// to avoid overwriting in-progress input.
			if (param.value.valid && lastSentValues?.current) {
				const sentValue = lastSentValues.current.get(param.name);
				const formValue = currentFormValuesMap.get(param.name);
				if (
					sentValue !== undefined &&
					param.value.value === sentValue &&
					formValue !== undefined &&
					formValue !== sentValue
				) {
					return { name: param.name, value: formValue };
				}
			}

			return {
				name: param.name,
				value: param.value.valid ? param.value.value : "",
			};
		});

		const isChanged =
			currentFormValues.length !== newParameterValues.length ||
			newParameterValues.some(
				(p) =>
					!currentFormValuesMap.has(p.name) ||
					currentFormValuesMap.get(p.name) !== p.value,
			);

		if (isChanged) {
			setFieldValue("rich_parameter_values", newParameterValues);
		}
	}, [parameters, setFieldValue]);
}
