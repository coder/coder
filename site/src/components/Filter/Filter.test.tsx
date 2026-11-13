import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { mockApiError } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { Filter, type UseFilterResult } from "./Filter";

const mockFilter: UseFilterResult = {
	query: "",
	values: {},
	used: false,
	update: () => {},
	debounceUpdate: () => {},
	cancelDebounce: () => {},
};

describe(Filter.name, () => {
	it("announces that the preset menu docs links open in a new tab", async () => {
		const user = userEvent.setup();
		render(
			<Filter
				filter={mockFilter}
				isLoading={false}
				optionsSkeleton={null}
				presets={[{ name: "All", query: "" }]}
				learnMoreLink="https://docs.example.com/filtering"
				learnMoreLabel2="User status"
				learnMoreLink2="https://docs.example.com/status"
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Filters" }));

		expect(
			await screen.findByRole("menuitem", { name: /^View advanced filtering/ }),
		).toHaveAccessibleName(/^View advanced filtering\s*\(opens in new tab\)$/);
		expect(
			screen.getByRole("menuitem", { name: /^User status/ }),
		).toHaveAccessibleName(/^User status\s*\(opens in new tab\)$/);
	});

	it("describes the search box with the validation error", () => {
		const detail = 'Query param "resource_type" has invalid value';
		render(
			<Filter
				filter={mockFilter}
				isLoading={false}
				optionsSkeleton={null}
				presets={[]}
				error={mockApiError({
					message: "Invalid query parameters.",
					validations: [{ field: "resource_type", detail }],
				})}
			/>,
		);

		expect(
			screen.getByRole("textbox", { name: "Search" }),
		).toHaveAccessibleDescription(detail);
	});
});
