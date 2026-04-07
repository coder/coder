import { useEffect, useRef } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import type { PreviewParameter } from "#/api/typesGenerated";

type UseSyncFormParametersProps = {
	parameters: readonly PreviewParameter[];
	formValues: readonly TypesGen.WorkspaceBuildParameter[];
	setFieldValue: (
		field: string,
		value: TypesGen.WorkspaceBuildParameter[],
	) => void;
	// When false the WebSocket has not yet returned a response that
	// reflects the initial form values we sent (autofill / previous
	// build). Preserve existing form values so the sync hook does not
	// overwrite them with stale server defaults.
	initialParamsAcknowledged?: boolean;
};

export function useSyncFormParameters({
	parameters,
	formValues,
	setFieldValue,
	initialParamsAcknowledged = true,
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
			// When the server value is not valid (e.g., the initial
			// WebSocket response before any user input is sent),
			// preserve the current form value. This prevents the sync
			// hook from overwriting autofilled values (from the
			// previous build) with empty strings before the server
			// has had a chance to process them.
			//
			// Also preserve the form value when the server has not yet
			// acknowledged the initial parameters we sent. The first
			// WS response (id -1) carries template defaults which
			// would overwrite autofilled values from a previous build
			// or URL search params.
			if (!param.value.valid || !initialParamsAcknowledged) {
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
	}, [parameters, setFieldValue, initialParamsAcknowledged]);
}
