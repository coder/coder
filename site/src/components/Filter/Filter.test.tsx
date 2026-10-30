import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "#/testHelpers/renderHelpers";
import { Filter, type UseFilterResult } from "./Filter";

const filter: UseFilterResult = {
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
				filter={filter}
				isLoading={false}
				optionsSkeleton={null}
				presets={[{ name: "All", query: "" }]}
				learnMoreLink="https://coder.com/docs/filtering"
				learnMoreLabel2="User status"
				learnMoreLink2="https://coder.com/docs/status"
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Filters" }));

		const items = await screen.findAllByRole("menuitem");
		expect(items).toHaveLength(2);
		expect(items[0]).toHaveAccessibleName(
			/^View advanced filtering\s*\(opens in new tab\)$/,
		);
		expect(items[1]).toHaveAccessibleName(
			/^User status\s*\(opens in new tab\)$/,
		);
	});
});
