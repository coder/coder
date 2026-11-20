import { fireEvent, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ProviderIcon } from "./ProviderIcon";

it("falls back to the generic icon when the configured icon fails to load", () => {
	renderComponent(
		<ProviderIcon
			provider="openai-compat"
			icon="/icon/does-not-exist.svg"
			alt="Solstice"
		/>,
	);
	fireEvent.error(screen.getByRole("img", { name: "Solstice" }));
	expect(screen.queryByRole("img", { name: "Solstice" })).toBeNull();
});
