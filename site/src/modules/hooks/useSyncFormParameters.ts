import type * as TypesGen from "api/typesGenerated";
import type { PreviewParameter } from "api/typesGenerated";
import { useEffect, useRef } from "react";

type UseSyncFormParametersProps = {
	parameters: readonly PreviewParameter[];
	formValues: readonly TypesGen.WorkspaceBuildParameter[];
	touched: Record<string, unknown>;
	setFieldValue: (
		field: string,
		value: TypesGen.WorkspaceBuildParameter[],
	) => void;
};

export function useSyncFormParameters({
	parameters,
	formValues,
	touched,
	setFieldValue,
}: UseSyncFormParametersProps) {
	// Form values only needs to be updated when parameters change
	// Keep track of form values in a ref to avoid unnecessary updates to rich_parameter_values
	const formValuesRef = useRef(formValues);
	const touchedRef = useRef(touched);

	useEffect(() => {
		formValuesRef.current = formValues;
	}, [formValues]);

	useEffect(() => {
		touchedRef.current = touched;
	}, [touched]);

	useEffect(() => {
		if (!parameters) return;
		const currentFormValues = formValuesRef.current;
		const currentTouched = touchedRef.current;
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

			const serverValue = param.value.value;
			const existingValue = currentFormValuesMap.get(param.name);

			// If the user has edited this field and the server hasn't
			// echoed back the user's value yet, preserve the local
			// value. This prevents stale WS responses from clobbering
			// user input that hasn't round-tripped yet.
			if (
				currentTouched[param.name] &&
				existingValue !== undefined &&
				existingValue !== serverValue
			) {
				return { name: param.name, value: existingValue };
			}

			return {
				name: param.name,
				value: serverValue,
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
