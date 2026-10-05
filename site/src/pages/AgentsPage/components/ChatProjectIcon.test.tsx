import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import themes, { DEFAULT_THEME } from "#/theme";
import { ChatProjectIcon } from "./ChatProjectIcon";

const Wrapper: React.FC<React.PropsWithChildren> = ({ children }) => (
	<ThemeOverride theme={themes[DEFAULT_THEME]}>{children}</ThemeOverride>
);

describe("ChatProjectIcon", () => {
	it("falls back to the folder glyph when the icon fails to load", () => {
		render(<ChatProjectIcon project={{ icon: "/emojis/missing.png" }} />, {
			wrapper: Wrapper,
		});

		fireEvent.error(screen.getByRole("img"));

		expect(screen.queryByRole("img")).toBeNull();
	});
});
