import { screen } from "@testing-library/react";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { IconField } from "./IconField";

describe("IconField", () => {
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
});
