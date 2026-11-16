import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { render } from "#/testHelpers/renderHelpers";
import { ProviderFilter, useProviderFilterMenu } from "./ProviderFilter";

afterEach(() => {
	vi.restoreAllMocks();
});

const ProviderFilterHarness: React.FC = () => {
	const menu = useProviderFilterMenu({
		value: undefined,
		onChange: vi.fn(),
		enabled: true,
	});
	return <ProviderFilter menu={menu} />;
};

it("names the default provider icon unless the label names it or the icon is custom", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "getAIBridgeProviders").mockResolvedValue([
		{ name: "bedrock", type: "bedrock", display_name: "", icon: "" },
		{ name: "anthropic-prod", type: "anthropic", display_name: "", icon: "" },
		{
			name: "acme",
			type: "bedrock",
			display_name: "Acme",
			icon: "/emojis/1f9ea.png",
		},
	]);
	render(<ProviderFilterHarness />);
	await user.click(screen.getByRole("button", { name: "Select provider" }));

	const bedrock = await screen.findByRole("option", { name: /bedrock/ });
	expect(within(bedrock).getByRole("img")).toHaveAccessibleName("AWS Bedrock");
	expect(
		screen.getByRole("option", { name: /anthropic-prod/ }),
	).toHaveAccessibleName("anthropic-prod");
	expect(screen.getByRole("option", { name: /Acme/ })).toHaveAccessibleName(
		"Acme",
	);
});
