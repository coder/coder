import { render, screen } from "@testing-library/react";
import type { FormHelpers } from "#/utils/formUtils";
import { FormField } from "./FormField";

const field: FormHelpers = {
	name: "value",
	id: "value",
	value: "",
	onChange: () => {},
	onBlur: () => {},
	error: false,
};

describe(FormField.name, () => {
	it("applies password-manager ignore attributes when ignorePasswordManagers is set", () => {
		render(
			<FormField
				id="story-field"
				field={field}
				label="Provider name"
				ignorePasswordManagers
			/>,
		);

		const input = screen.getByRole("textbox", { name: /Provider name/ });
		expect(input).toHaveAttribute("autocomplete", "off");
		expect(input).toHaveAttribute("data-1p-ignore", "true");
		expect(input).toHaveAttribute("data-lpignore", "true");
		expect(input).toHaveAttribute("data-form-type", "other");
		expect(input).toHaveAttribute("data-bwignore", "true");
	});

	it("omits password-manager ignore attributes by default", () => {
		render(<FormField id="story-field" field={field} label="Provider name" />);

		const input = screen.getByRole("textbox", { name: /Provider name/ });
		expect(input).not.toHaveAttribute("data-1p-ignore");
		expect(input).not.toHaveAttribute("data-lpignore");
		expect(input).not.toHaveAttribute("data-form-type");
		expect(input).not.toHaveAttribute("data-bwignore");
	});
});
