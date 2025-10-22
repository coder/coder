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
	// Form values only needs to be updated when parameters change
	// Keep track of form values in a ref to avoid unnecessary updates to rich_parameter_values
	const formValuesRef = useRef(formValues);

	useEffect(() => {
		formValuesRef.current = formValues;
	}, [formValues]);

	useEffect(() => {
		if (!parameters) return;
		const currentFormValues = formValuesRef.current;

		const newParameterValues = parameters.map((param) => ({
			name: param.name,
			value: param.value.valid ? param.value.value : "",
		}));

		const currentFormValuesMap = new Map(
			currentFormValues.map((value) => [value.name, value.value]),
		);

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
