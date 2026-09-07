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
	// The effect must not depend on `formValues`: formik commits values and
	// touched separately, so a values-only commit would re-run the effect
	// before `touched` marks the edit, clobbering it with the server value.
	// The ref carries the latest committed values into the next run instead.
	const formValuesRef = useRef(formValues);

	useEffect(() => {
		// Mirror post-commit so the next run sees the latest values without
		// adding them to the dependency list.
		formValuesRef.current = formValues;
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
