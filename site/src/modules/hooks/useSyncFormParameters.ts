import { useEffect, useEffectEvent } from "react";
import type {
	PreviewParameter,
	WorkspaceBuildParameter,
} from "#/api/typesGenerated";

type UseSyncFormParametersProps = {
	parameters: readonly PreviewParameter[];
	formValues: readonly WorkspaceBuildParameter[];
	/**
	 * Reports whether the user (or autofill) has set a parameter's value. Edited
	 * values are kept as they are instead of being replaced by the response.
	 */
	isParameterEdited: (name: string) => boolean;
	setFieldValue: (field: string, value: WorkspaceBuildParameter[]) => void;
};

export function useSyncFormParameters({
	parameters,
	formValues,
	isParameterEdited,
	setFieldValue,
}: UseSyncFormParametersProps) {
	// Form values only needs to be updated when parameters change. Reading the
	// latest form values from an effect event keeps them out of the effect's
	// dependency array so it does not re-run on every form value change.
	const syncParameters = useEffectEvent(() => {
		const currentFormValuesMap = new Map(
			formValues.map((value) => [value.name, value.value]),
		);

		const newParameterValues = parameters.map((param) => {
			// Do not mess with values the user has changed (or were auto-filled).
			// Otherwise based on timing web socket responses can undo changes, and it
			// seems bad to change a user's inputs from under them anyway.
			if (isParameterEdited(param.name)) {
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
			formValues.length !== newParameterValues.length ||
			newParameterValues.some(
				(p) =>
					!currentFormValuesMap.has(p.name) ||
					currentFormValuesMap.get(p.name) !== p.value,
			);

		if (isChanged) {
			setFieldValue("rich_parameter_values", newParameterValues);
		}
	});

	useEffect(() => {
		if (!parameters) return;
		syncParameters();
	}, [parameters]);
}
