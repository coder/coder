import type * as TypesGen from "api/typesGenerated";
import { useEffect, useRef } from "react";

import type { PreviewParameter } from "api/typesGenerated";

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

		const newParameterValues = parameters.map((param) => {
			return {
				name: param.name,
				value: param.value.valid ? param.value.value : "",
			};
		});

		const isChanged =
			currentFormValues.length !== newParameterValues.length ||
			newParameterValues.some(
				(p) =>
					!currentFormValues.find(
						(formValue) =>
							formValue.name === p.name && formValue.value === p.value,
					),
			);

		if (isChanged) {
			setFieldValue("rich_parameter_values", newParameterValues);
		}
	}, [parameters, setFieldValue]);
}
