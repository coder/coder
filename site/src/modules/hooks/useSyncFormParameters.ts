import type { FormikTouched } from "formik";
import { useEffect, useRef } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import type { PreviewParameter } from "#/api/typesGenerated";
import {
	type AutofillBuildParameter,
	isValidParameterOption,
} from "#/utils/richParameters";

type UseSyncFormParametersProps = {
	parameters: readonly PreviewParameter[];
	autofillParameters: readonly AutofillBuildParameter[];
	formValues: readonly TypesGen.WorkspaceBuildParameter[];
	touched: FormikTouched<{
		rich_parameter_values?: readonly TypesGen.WorkspaceBuildParameter[];
	}>;
	setFieldValue: (
		field: string,
		value: TypesGen.WorkspaceBuildParameter[],
	) => void;
	setFieldTouched: (
		field: string,
		isTouched?: boolean,
		shouldValidate?: boolean,
	) => void;
};

export function useSyncFormParameters({
	parameters,
	autofillParameters,
	formValues,
	touched,
	setFieldValue,
	setFieldTouched,
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
		const autofillByName = new Map(
			autofillParameters.map((value) => [value.name, value]),
		);
		const newlyAutofilledParameters: string[] = [];

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

			const autofillParameter = autofillByName.get(param.name);
			if (
				!currentFormValuesMap.has(param.name) &&
				!param.ephemeral &&
				autofillParameter &&
				isValidParameterOption(param, autofillParameter)
			) {
				newlyAutofilledParameters.push(param.name);
				return { name: param.name, value: autofillParameter.value };
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
			for (const parameterName of newlyAutofilledParameters) {
				setFieldTouched(parameterName, true, false);
			}
		}
	}, [parameters, autofillParameters, touched, setFieldValue, setFieldTouched]);
}
