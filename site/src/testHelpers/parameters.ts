import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type * as TypesGen from "#/api/typesGenerated";

type Parameter = TypesGen.WorkspaceBuildParameter | TypesGen.PreviewParameter;

export function isBuildParameter(
	parameter: Parameter,
): parameter is TypesGen.WorkspaceBuildParameter {
	return typeof parameter.value === "string";
}

// checkParameters waits until all the provided parameters have the expected
// display value within the parameters form.  Requires that the form and
// parameters all have test IDs (`form` and `parameter-field-$name`).
export async function checkParameters(...parameters: Parameter[]) {
	const form = screen.getByTestId("form");
	await waitFor(() => {
		for (const parameter of parameters) {
			const field = within(form).getByTestId(
				`parameter-field-${parameter.name}`,
			);
			const value = isBuildParameter(parameter)
				? parameter.value
				: parameter.value.value;
			expect(within(field).getByDisplayValue(value)).toBeInTheDocument();
		}
	});
}

// editParameters edits each parameter so it has the provided value.  Requires
// that the form and parameters all have test IDs (`form` and
// `parameter-field-$name`).
export async function editParameters(...parameters: Parameter[]) {
	const form = screen.getByTestId("form");
	for (const parameter of parameters) {
		const field = within(form).getByTestId(`parameter-field-${parameter.name}`);
		const input = await within(field).findByRole("textbox", {
			name: new RegExp(parameter.name, "i"),
		});
		await userEvent.clear(input);
		const value = isBuildParameter(parameter)
			? parameter.value
			: parameter.value.value;
		if (value !== "") {
			await userEvent.type(input, value);
		}
	}
}
