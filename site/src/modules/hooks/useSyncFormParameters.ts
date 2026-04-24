import type { FormikTouched } from "formik";
import { useEffect, useRef } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import type { PreviewParameter } from "#/api/typesGenerated";

type UseSyncFormParametersProps = {
	parameters: readonly PreviewParameter[];
	formValues: readonly TypesGen.WorkspaceBuildParameter[];
	touched: FormikTouched<{
		rich_parameter_values?: readonly TypesGen.WorkspaceBuildParameter[];
	}>;
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

	formValuesRef.current = formValues;

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
			if (
				touched[
					param.name as keyof {
						rich_parameter_values?: readonly TypesGen.WorkspaceBuildParameter[];
					}
				]
			) {
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
	}, [parameters, touched, setFieldValue]);
}
