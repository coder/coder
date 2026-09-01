import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { IconField } from "./IconField";

const meta: Meta<typeof IconField> = {
	title: "components/IconField",
	component: IconField,
	args: {
		onPickEmoji: fn(),
		onChange: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof IconField>;

const openPicker = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	const button = canvas.getByRole("button", { name: "Pick an emoji or icon" });
	await userEvent.click(button);
	await expect(button).toHaveAttribute("aria-expanded", "true");
	const dialog = await screen.findByRole("dialog");
	await waitFor(() => {
		expect(
			within(dialog).queryByRole("status", { name: "Loading" }),
		).not.toBeInTheDocument();
	});
	return dialog;
};

const grid = (dialog: HTMLElement) =>
	within(dialog).getByRole("listbox", { name: "Emojis and icons" });

const search = async (dialog: HTMLElement, query: string) => {
	const input = within(dialog).getByRole("combobox", {
		name: "Search emojis and icons",
	});
	await userEvent.clear(input);
	await userEvent.type(input, query);
	return input;
};

const chooseCategory = async (dialog: HTMLElement, name: string) => {
	await userEvent.click(
		within(dialog).getByRole("combobox", { name: /^Category:/ }),
	);
	await userEvent.click(await screen.findByRole("option", { name }));
};

const chooseTone = async (dialog: HTMLElement, label: string) => {
	await userEvent.click(
		within(dialog).getByRole("combobox", { name: /^Skin tone:/ }),
	);
	await userEvent.click(await screen.findByRole("option", { name: label }));
	await expect(
		within(dialog).getByRole("combobox", { name: `Skin tone: ${label}` }),
	).toBeVisible();
};

const announcements = (dialog: HTMLElement) =>
	within(dialog)
		.getAllByRole("status")
		.map((region) => region.textContent);

export const Example: Story = {};

export const EmojiSelected: Story = {
	args: {
		value: "/emojis/1f3f3-fe0f-200d-26a7-fe0f.png",
	},
};

export const IconSelected: Story = {
	args: {
		value: "/icon/fedora.svg",
	},
};

export const WithHelperText: Story = {
	args: {
		helperText: "Paste an image URL or pick an emoji.",
	},
};

export const WithError: Story = {
	args: {
		error: true,
		helperText: "Icon URL is too long.",
		value: "https://example.com/very-long-icon-url.png",
	},
};

export const OpenPicker: Story = {
	play: async ({ canvasElement }) => {
		const dialog = await openPicker(canvasElement);
		const list = grid(dialog);
		const grinning = within(list).getByRole("option", {
			name: "Grinning face",
		});
		await expect(grinning).toBeVisible();
		await expect(within(grinning).getByTestId("emoji-sprite")).toBeVisible();
		await expect(
			within(list).queryByRole("option", { name: "fedora" }),
		).not.toBeInTheDocument();
	},
};

export const BrowseCategory: Story = {
	play: async ({ canvasElement }) => {
		const dialog = await openPicker(canvasElement);
		await chooseCategory(dialog, "Coder icons");

		const list = grid(dialog);
		const fedora = within(list).getByRole("option", { name: "fedora" });
		await expect(fedora).toBeVisible();
		await expect(within(fedora).queryByTestId("emoji-sprite")).toBeNull();
		await expect(within(fedora).getByAltText("")).toHaveAttribute(
			"src",
			"/icon/fedora.svg",
		);
		await expect(
			within(list).queryByRole("option", { name: "Grinning face" }),
		).not.toBeInTheDocument();
	},
};

export const ReselectingCategoryClearsSearch: Story = {
	play: async ({ canvasElement }) => {
		const dialog = await openPicker(canvasElement);
		const input = await search(dialog, "fedora");
		await expect(
			await within(dialog).findByRole("option", { name: "fedora" }),
		).toBeVisible();

		await expect(
			within(dialog).getByRole("combobox", {
				name: "Category: Search results",
			}),
		).toBeVisible();
		await chooseCategory(dialog, "Smileys & Emotion");

		await expect(input).toHaveValue("");
		await expect(
			within(dialog).getByRole("combobox", {
				name: "Category: Smileys & Emotion",
			}),
		).toBeVisible();
		await expect(
			within(grid(dialog)).getByRole("option", { name: "Grinning face" }),
		).toBeVisible();
		await expect(
			within(dialog).queryByRole("option", { name: "fedora" }),
		).not.toBeInTheDocument();
	},
};

export const CancellingCategoryPreservesSearch: Story = {
	play: async ({ canvasElement }) => {
		const dialog = await openPicker(canvasElement);
		const input = await search(dialog, "fedora");
		await userEvent.click(
			within(dialog).getByRole("combobox", { name: /^Category:/ }),
		);
		await userEvent.keyboard("{Escape}");

		await expect(input).toHaveValue("fedora");
		await expect(
			within(dialog).getByRole("combobox", {
				name: "Category: Search results",
			}),
		).toBeVisible();
		await expect(
			within(grid(dialog)).getByRole("option", { name: "fedora" }),
		).toBeVisible();
	},
};

export const SearchAndSelectEmoji: Story = {
	play: async ({ canvasElement, args }) => {
		const dialog = await openPicker(canvasElement);
		await search(dialog, "waving hand");
		await userEvent.click(
			await within(dialog).findByRole("option", { name: "Waving hand" }),
		);

		await expect(args.onPickEmoji).toHaveBeenCalledWith("/emojis/1f44b.png");
		await waitFor(() =>
			expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
		);
	},
};

export const SearchRanksExactMatchFirst: Story = {
	play: async ({ canvasElement }) => {
		const dialog = await openPicker(canvasElement);
		await search(dialog, "wave");

		const list = grid(dialog);
		await waitFor(async () => {
			const [first] = within(list).getAllByRole("option");
			await expect(first).toHaveAccessibleName("Waving hand");
		});
		await expect(
			within(list).getByRole("option", { name: "Water wave" }),
		).toBeVisible();
	},
};

export const SearchByKeyword: Story = {
	play: async ({ canvasElement }) => {
		const dialog = await openPicker(canvasElement);
		// "haha" appears in the keyword list only, never in a name or id.
		await search(dialog, "haha");

		const list = grid(dialog);
		await waitFor(async () => {
			const [first] = within(list).getAllByRole("option");
			await expect(first).toHaveAccessibleName("Grinning face with big eyes");
		});
	},
};

export const SearchByTextAlias: Story = {
	play: async ({ canvasElement }) => {
		const dialog = await openPicker(canvasElement);
		await search(dialog, ":D");

		const list = grid(dialog);
		await waitFor(async () => {
			const [first] = within(list).getAllByRole("option");
			await expect(first).toHaveAccessibleName("Grinning face");
		});
	},
};

export const SearchByAlias: Story = {
	play: async ({ canvasElement }) => {
		const dialog = await openPicker(canvasElement);
		await search(dialog, "satisfied");

		const list = grid(dialog);
		await waitFor(async () => {
			const [first] = within(list).getAllByRole("option");
			await expect(first).toHaveAccessibleName("Grinning squinting face");
		});
	},
};

export const SearchTermsAreOrderIndependent: Story = {
	play: async ({ canvasElement }) => {
		const dialog = await openPicker(canvasElement);
		await search(dialog, "hand waving");

		await expect(
			await within(dialog).findByRole("option", { name: "Waving hand" }),
		).toBeVisible();
	},
};

export const SearchRanksCoderIconsAboveLooseMatches: Story = {
	play: async ({ canvasElement, args }) => {
		const dialog = await openPicker(canvasElement);
		await search(dialog, "code");

		const list = grid(dialog);
		await waitFor(async () => {
			const [first, second] = within(list).getAllByRole("option");
			await expect(first).toHaveAccessibleName("code");
			await expect(second).toHaveAccessibleName("code-insiders");
		});
		await expect(
			within(list).getByRole("option", { name: "Technologist" }),
		).toBeVisible();

		await userEvent.click(within(list).getByRole("option", { name: "code" }));
		await expect(args.onPickEmoji).toHaveBeenCalledWith("/icon/code.svg");
	},
};

export const SelectCustomIcon: Story = {
	play: async ({ canvasElement, args }) => {
		const dialog = await openPicker(canvasElement);
		await search(dialog, "fedora");
		await userEvent.click(
			await within(dialog).findByRole("option", { name: "fedora" }),
		);

		await expect(args.onPickEmoji).toHaveBeenCalledWith("/icon/fedora.svg");
	},
};

export const SelectEmojiWithKeyboard: Story = {
	play: async ({ canvasElement, args }) => {
		const dialog = await openPicker(canvasElement);
		const input = await search(dialog, "waving hand");
		await expect(input).toHaveFocus();
		await userEvent.keyboard("{Enter}");

		await expect(args.onPickEmoji).toHaveBeenCalledWith("/emojis/1f44b.png");
	},
};

export const HorizontalArrowsEditSearch: Story = {
	play: async ({ canvasElement }) => {
		const dialog = await openPicker(canvasElement);
		const input = await search(dialog, "wae");
		await userEvent.keyboard("{ArrowLeft}v");

		await expect(input).toHaveValue("wave");
		await expect(
			within(grid(dialog)).getByRole("option", { name: "Waving hand" }),
		).toBeVisible();

		await userEvent.keyboard("{Home}g{End}!");
		await expect(input).toHaveValue("gwave!");
	},
};

export const NavigateGridWithArrowKeys: Story = {
	play: async ({ canvasElement, args }) => {
		const dialog = await openPicker(canvasElement);
		const input = within(dialog).getByRole("combobox", {
			name: "Search emojis and icons",
		});
		const list = grid(dialog);
		const selected = () => within(list).getByRole("option", { selected: true });
		const expectActiveOption = async (name: string) => {
			await expect(selected()).toHaveAccessibleName(name);
			await expect(input).toHaveAttribute(
				"aria-activedescendant",
				selected().id,
			);
		};

		await waitFor(() => expectActiveOption("Grinning face"));

		await userEvent.keyboard("{ArrowRight}");
		await waitFor(() => expectActiveOption("Grinning face with big eyes"));

		// One row down from the second cell is the tenth option in an 8 wide grid.
		await userEvent.keyboard("{ArrowDown}");
		await waitFor(() => expectActiveOption("Upside-down face"));

		await userEvent.keyboard("{ArrowLeft}");
		await waitFor(() => expectActiveOption("Slightly smiling face"));

		await userEvent.keyboard("{ArrowUp}");
		await waitFor(() => expectActiveOption("Grinning face"));

		await userEvent.keyboard("{Enter}");
		await expect(args.onPickEmoji).toHaveBeenCalledWith("/emojis/1f600.png");
	},
};

export const SelectExactSkinToneVariant: Story = {
	play: async ({ canvasElement, args }) => {
		const dialog = await openPicker(canvasElement);
		await search(dialog, "waving hand");
		const wavingHand = await within(dialog).findByRole("option", {
			name: "Waving hand",
		});
		const sprite = within(wavingHand).getByTestId("emoji-sprite");
		const defaultTonePosition = sprite.style.backgroundPosition;
		await chooseTone(dialog, "Dark");
		await expect(sprite.style.backgroundPosition).not.toBe(defaultTonePosition);
		await userEvent.click(wavingHand);

		await expect(args.onPickEmoji).toHaveBeenCalledWith(
			"/emojis/1f44b-1f3ff.png",
		);
	},
};

export const SelectCompositeSkinToneVariant: Story = {
	play: async ({ canvasElement, args }) => {
		const dialog = await openPicker(canvasElement);
		await chooseTone(dialog, "Dark");
		// Multi-person emoji ship one image per tone pair, so the picker falls
		// back to the pair that repeats the chosen tone.
		await search(dialog, "people holding hands");
		await userEvent.click(
			await within(dialog).findByRole("option", {
				name: "People holding hands",
			}),
		);

		await expect(args.onPickEmoji).toHaveBeenCalledWith(
			"/emojis/1f9d1-1f3ff-200d-1f91d-200d-1f9d1-1f3ff.png",
		);
	},
};

export const SkinToneFallsBackToTonelessEmoji: Story = {
	play: async ({ canvasElement, args }) => {
		const dialog = await openPicker(canvasElement);
		await chooseTone(dialog, "Dark");
		await search(dialog, "grinning face");
		await userEvent.click(
			await within(dialog).findByRole("option", { name: "Grinning face" }),
		);

		await expect(args.onPickEmoji).toHaveBeenCalledWith("/emojis/1f600.png");
	},
};

export const SearchWithNoResults: Story = {
	play: async ({ canvasElement }) => {
		const dialog = await openPicker(canvasElement);
		await search(dialog, "nothingmatchesthis");

		await expect(
			await within(dialog).findByText("No emojis or icons match your search."),
		).toBeVisible();
		await expect(announcements(dialog)).toContain(
			"No emojis or icons match your search.",
		);
		await expect(within(dialog).queryAllByRole("option")).toHaveLength(0);
	},
};

export const SearchResultsAreCapped: Story = {
	play: async ({ canvasElement }) => {
		const dialog = await openPicker(canvasElement);
		await search(dialog, "face");

		await expect(
			await within(dialog).findByText(
				"Showing the first 64 matches. Keep typing to narrow the search.",
			),
		).toBeVisible();
		await expect(announcements(dialog)).toContain(
			"Showing the first 64 matches. Keep typing to narrow the search.",
		);
		await expect(within(dialog).getAllByRole("option")).toHaveLength(64);
	},
};
