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
	// IconField lazy-loads the picker. The first import evaluates the emoji
	// data, which can outlast findByRole's timeout on a busy CI runner, so load
	// it before any test opens the picker.
	beforeAll(async () => {
		await import("./EmojiPicker");
	});

	it.each([
		["/emojis/1f524.png", "🔤"],
		["/emojis/1f9d1-200d-1f4bb.png", "🧑‍💻"],
	])("names the %s emoji preview", (value, name) => {
		renderComponent(
			<IconField value={value} onChange={vi.fn()} onPickEmoji={vi.fn()} />,
		);

		expect(screen.getByRole("img")).toHaveAccessibleName(name);
	});

	it("keeps the preview decorative for an invalid emoji code point", () => {
		renderComponent(
			<IconField
				value="/emojis/ffffffff.png"
				onChange={vi.fn()}
				onPickEmoji={vi.fn()}
			/>,
		);

		expect(screen.getByRole("img")).toHaveAccessibleName("");
	});

	it.each([
		["docker", "Docker", "/icon/docker.svg"],
		["Kubernetes", "Kubernetes", "/icon/k8s.svg"],
		["k8s", "Kubernetes", "/icon/k8s.svg"],
		["Fly.io", "Fly.io", "/icon/fly.io.svg"],
		["aws-dark", "AWS dark", "/icon/aws-dark.svg"],
		["aws light", "AWS light", "/icon/aws-light.svg"],
		["gemini-monochrome", "Gemini monochrome", "/icon/gemini-monochrome.svg"],
	])("picks a custom icon found by %s", async (query, label, value) => {
		const onPickEmoji = vi.fn();
		renderComponent(<IconField onPickEmoji={onPickEmoji} />);
		const { user, search } = await openPicker();

		await user.type(search, query);
		await user.click(await screen.findByRole("gridcell", { name: label }));

		expect(onPickEmoji).toHaveBeenCalledWith(value);
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
		await screen.findByRole("gridcell", { name: "Docker" });
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

	// ARIA grid indexes start at 1; Frimousse 0.4.0 emits 0-based ones, so
	// site/patches/frimousse@0.4.0.patch shifts them.
	it("numbers grid rows and cells from 1", async () => {
		renderComponent(<IconField onPickEmoji={vi.fn()} />);
		const { user, search } = await openPicker();

		await user.type(search, "docker");
		const cell = await screen.findByRole("gridcell", { name: "Docker" });

		expect(cell).toHaveAttribute("aria-colindex", "1");
		expect(screen.getByRole("row")).toHaveAttribute("aria-rowindex", "1");
	});
});
