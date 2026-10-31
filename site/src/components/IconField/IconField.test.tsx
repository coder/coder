import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { IconField } from "./IconField";

const openPicker = async () => {
	const user = userEvent.setup();
	await user.click(
		screen.getByRole("button", { name: "Pick an emoji or icon" }),
	);
	const search = await screen.findByRole("searchbox", {
		name: "Search emojis and icons",
	});
	return { user, search };
};

describe("IconField", () => {
	it("picks a custom icon found by search", async () => {
		const onPickEmoji = vi.fn();
		renderComponent(<IconField onPickEmoji={onPickEmoji} />);
		const { user, search } = await openPicker();

		await user.type(search, "docker");
		await user.click(await screen.findByRole("gridcell", { name: "docker" }));

		expect(onPickEmoji).toHaveBeenCalledWith("/icon/docker.svg");
	});

	it("picks a Unicode emoji as its Apple image path", async () => {
		const onPickEmoji = vi.fn();
		renderComponent(<IconField onPickEmoji={onPickEmoji} />);
		const { user, search } = await openPicker();

		await user.type(search, "men wrestling");
		await user.click(
			await screen.findByRole("gridcell", { name: "men wrestling" }),
		);

		expect(onPickEmoji).toHaveBeenCalledWith(
			"/emojis/1f93c-200d-2642-fe0f.png",
		);
	});

	it("picks a search result with the keyboard", async () => {
		const onPickEmoji = vi.fn();
		renderComponent(<IconField onPickEmoji={onPickEmoji} />);
		const { user, search } = await openPicker();

		await user.type(search, "docker");
		await screen.findByRole("gridcell", { name: "docker" });
		await user.keyboard("{ArrowDown}{Enter}");

		expect(onPickEmoji).toHaveBeenCalledWith("/icon/docker.svg");
	});

	it("clears the search", async () => {
		renderComponent(<IconField onPickEmoji={vi.fn()} />);
		const { user, search } = await openPicker();

		await user.type(search, "docker");
		await user.click(screen.getByRole("button", { name: "Clear search" }));

		expect(search).toHaveValue("");
	});

	it("marks a category as current when it is picked", async () => {
		renderComponent(<IconField onPickEmoji={vi.fn()} />);
		const { user } = await openPicker();
		const iconsCategory = screen.getByRole("button", { name: "Icons" });

		await user.click(iconsCategory);

		expect(iconsCategory).toHaveAttribute("aria-current", "true");
	});
});
