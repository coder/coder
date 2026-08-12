import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type * as TypesGen from "#/api/typesGenerated";

type BuildParameter = TypesGen.WorkspaceBuildParameter & {
	display_name?: string;
	form_type?: TypesGen.ParameterFormType;
};

type Parameter = BuildParameter | TypesGen.PreviewParameter;

export function isBuildParameter(
	parameter: Parameter,
): parameter is TypesGen.WorkspaceBuildParameter {
	return typeof parameter.value === "string";
}

// checkParameters waits until all the provided parameters have the expected
// display value within the parameters form and that there are no additional
// parameters.  Requires that the form and parameters all have test IDs (`form`
// and `parameter-field-$name`).
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
			const type = parameter.form_type || "input";
			switch (type) {
				case "switch":
					if (value === "true") {
						expect(within(field).getByRole("switch")).toBeChecked();
					} else {
						expect(within(field).getByRole("switch")).not.toBeChecked();
					}
					break;
				case "dropdown":
					expect(within(field).getByRole("combobox")).toHaveTextContent(
						value || "Select option",
					);
					break;
				case "multi-select":
				case "tag-select":
					// TODO: Validate these values as well, not just that they exist.
					break;
				case "input":
				case "slider":
					expect(within(field).getByDisplayValue(value)).toBeInTheDocument();
					break;
				default:
					throw new Error(`checking ${type} fields is not implemented`);
			}
		}
		const fields = within(form).queryAllByTestId(/^parameter-field-/);
		expect(fields).toHaveLength(parameters.length);
	});
}

// editParameters edits each parameter so it has the provided value.  Requires
// that the form and parameters all have test IDs (`form` and
// `parameter-field-$name`).
export async function editParameters(...parameters: Parameter[]) {
	const form = screen.getByTestId("form");
	for (const parameter of parameters) {
		const field = within(form).getByTestId(`parameter-field-${parameter.name}`);
		const value = isBuildParameter(parameter)
			? parameter.value
			: parameter.value.value;
		const type = parameter.form_type || "input";
		const label = parameter.display_name || parameter.name;
		switch (type) {
			case "input": {
				const input = await within(field).findByLabelText(
					new RegExp(label, "i"),
				);
				await userEvent.clear(input);
				if (value !== "") {
					await userEvent.type(input, value);
				}
				break;
			}
			default:
				throw new Error(`editing ${type} fields is not implemented`);
		}
	}
}
