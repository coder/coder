import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockOrganization, MockOrganization2 } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { MockResizeObserver } from "#/testHelpers/resizeObserver";
import { OrganizationAutocomplete } from "./OrganizationAutocomplete";

beforeEach(() => {
	MockResizeObserver.reset();
	vi.stubGlobal("ResizeObserver", MockResizeObserver);
});

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

/** jsdom has no layout, so the test sets the width every label measures. */
const stubLabelWidths = (widths: {
	scrollWidth: number;
	clientWidth: number;
}) => {
	vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(
		widths.scrollWidth,
	);
	vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(
		widths.clientWidth,
	);
};

const openAutocomplete = async ({ optionsTabbable = false } = {}) => {
	const user = userEvent.setup();
	renderComponent(
		<OrganizationAutocomplete
			ariaLabel="Organization"
			value={null}
			onChange={vi.fn()}
			options={[MockOrganization, MockOrganization2]}
			optionsTabbable={optionsTabbable}
		/>,
	);
	await user.click(getTrigger());
	await screen.findAllByRole("option");
	return user;
};

const getTrigger = () => screen.getByRole("combobox", { name: "Organization" });

// The avatar fallback initials are part of each option's accessible name.
const getOption = (name: string) =>
	screen.getByRole("option", { name: new RegExp(`${name}$`) });

describe("OrganizationAutocomplete", () => {
	it("shows a truncated name in a tooltip for the keyboard-highlighted option", async () => {
		stubLabelWidths({ scrollWidth: 200, clientWidth: 100 });
		const user = await openAutocomplete();

		await user.keyboard("{ArrowDown}");

		await waitFor(() =>
			expect(
				getOption(MockOrganization2.display_name),
			).toHaveAccessibleDescription(MockOrganization2.display_name),
		);
	});

	it("shows a truncated name in a tooltip for a tabbed-to option", async () => {
		stubLabelWidths({ scrollWidth: 200, clientWidth: 100 });
		const user = await openAutocomplete({ optionsTabbable: true });

		await user.tab();
		await user.tab();

		const option = getOption(MockOrganization2.display_name);
		expect(option).toHaveFocus();
		await waitFor(() =>
			expect(option).toHaveAccessibleDescription(
				MockOrganization2.display_name,
			),
		);
	});

	it("adds no tooltip to an option whose name fits", async () => {
		stubLabelWidths({ scrollWidth: 100, clientWidth: 100 });
		const user = await openAutocomplete();

		await user.keyboard("{ArrowDown}");

		expect(
			getOption(MockOrganization2.display_name),
		).not.toHaveAccessibleDescription();
	});

	it("closes the tooltip, then the list, on Escape", async () => {
		stubLabelWidths({ scrollWidth: 200, clientWidth: 100 });
		const user = await openAutocomplete();
		const option = getOption(MockOrganization.display_name);
		await waitFor(() =>
			expect(option).toHaveAccessibleDescription(MockOrganization.display_name),
		);

		await user.keyboard("{Escape}");
		await waitFor(() => expect(option).not.toHaveAccessibleDescription());
		expect(getTrigger()).toHaveAttribute("aria-expanded", "true");

		await user.keyboard("{Escape}");
		expect(getTrigger()).toHaveAttribute("aria-expanded", "false");
	});
});
