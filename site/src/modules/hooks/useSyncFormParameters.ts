import type * as TypesGen from "api/typesGenerated";
import type { PreviewParameter } from "api/typesGenerated";
import { useEffect, useRef } from "react";

type UseSyncFormParametersProps = {
	parameters: readonly PreviewParameter[];
	formValues: readonly TypesGen.WorkspaceBuildParameter[];
	setFieldValue: (
		field: string,
		value: TypesGen.WorkspaceBuildParameter[],
	) => void;
};

export function useSyncFormParameters({
	parameters,
	formValues,
	setFieldValue,
}: UseSyncFormParametersProps) {
	// Form values only needs to be updated when parameters change.
	// Keep track of form values in a ref to avoid unnecessary
	// updates to rich_parameter_values.
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
			// When the server has not evaluated this parameter yet
			// (valid=false), its value is meaningless — preserve
			// whatever the form already holds (e.g. autofill or
			// previous build values). When valid=true, the server
			// has intentionally set this value and we respect it.
			if (!param.value.valid) {
				const currentValue = currentFormValuesMap.get(param.name);
				if (currentValue) {
					return { name: param.name, value: currentValue };
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
