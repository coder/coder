import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { TemplateVersionVariable } from "#/api/typesGenerated";
import {
	MockTemplateVersionVariable1,
	MockTemplateVersionVariable2,
} from "#/testHelpers/entities";
import { MissingTemplateVariablesDialog } from "./MissingTemplateVariablesDialog";

describe("MissingTemplateVariablesDialog", () => {
	it("submits the values of variables that load after opening", async () => {
		const onSubmit = vi.fn();
		const dialog = (missingVariables?: TemplateVersionVariable[]) => (
			<MissingTemplateVariablesDialog
				open
				onClose={vi.fn()}
				onSubmit={onSubmit}
				missingVariables={missingVariables}
			/>
		);
		const { rerender } = render(dialog());

		rerender(
			dialog([
				{ ...MockTemplateVersionVariable1, value: "first" },
				{ ...MockTemplateVersionVariable2, value: "second" },
			]),
		);
		await userEvent.click(screen.getByRole("button", { name: "Submit" }));

		expect(onSubmit).toHaveBeenCalledWith([
			{ name: MockTemplateVersionVariable1.name, value: "first" },
			{ name: MockTemplateVersionVariable2.name, value: "second" },
		]);
	});
});
