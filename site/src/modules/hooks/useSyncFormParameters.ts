import type * as TypesGen from "api/typesGenerated";
import type { PreviewParameter } from "api/typesGenerated";
import { useEffect, useRef } from "react";

type UseSyncFormParametersProps = {
	parameters: readonly PreviewParameter[];
	formValues: readonly TypesGen.WorkspaceBuildParameter[];
	protectedParameters?: ReadonlySet<string>;
	setFieldValue: (
		field: string,
		value: TypesGen.WorkspaceBuildParameter[],
	) => void;
};

export function useSyncFormParameters({
	parameters,
	formValues,
	protectedParameters,
	setFieldValue,
}: UseSyncFormParametersProps) {
	// Form values only needs to be updated when parameters change
	// Keep track of form values in a ref to avoid unnecessary updates to rich_parameter_values
	const formValuesRef = useRef(formValues);

	useEffect(() => {
		formValuesRef.current = formValues;
	}, [formValues]);

	useEffect(() => {
		if (!parameters) return;
		const currentFormValues = formValuesRef.current;
		const currentFormValuesMap = new Map(
			currentFormValues.map((value) => [value.name, value.value]),
		);

		const newParameterValues = parameters.map((param) => {
			// Do not mess with values the user has changed (or were auto-filled).
			// Otherwise based on timing web socket responses can undo changes, and it
			// seems bad to change a user's inputs from under them anyway.
			const isProtected =
				!param.value.valid || protectedParameters?.has(param.name);
			if (isProtected) {
				const existingValue = currentFormValuesMap.get(param.name);
				if (existingValue !== undefined) {
					return { name: param.name, value: existingValue };
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
	}, [parameters, protectedParameters, setFieldValue]);
}
