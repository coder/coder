import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { render } from "#/testHelpers/renderHelpers";
import { ModelFilter, useModelFilterMenu } from "./ModelFilter";

afterEach(() => {
	vi.restoreAllMocks();
});

const bedrockModel = "us.anthropic.claude-3-5-sonnet-20241022-v2:0";

const ModelFilterHarness: FC<{ value?: string }> = ({ value }) => {
	const menu = useModelFilterMenu({ value, onChange: vi.fn(), enabled: true });
	return <ModelFilter menu={menu} />;
};

it("looks up the selected model as a quoted literal", async () => {
	const modelsSpy = vi
		.spyOn(API, "getAIBridgeModels")
		.mockResolvedValue([bedrockModel]);
	render(<ModelFilterHarness value={bedrockModel} />);
	await waitFor(() =>
		expect(modelsSpy).toHaveBeenCalledWith({
			q: `model:"${bedrockModel}"`,
			limit: 1,
		}),
	);
});

it("searches typed text as a quoted literal with LIKE wildcards escaped", async () => {
	const user = userEvent.setup();
	const modelsSpy = vi.spyOn(API, "getAIBridgeModels").mockResolvedValue([]);
	render(<ModelFilterHarness />);
	await user.click(screen.getByRole("button", { name: "Select model" }));
	await user.type(await screen.findByRole("combobox"), "gpt_4%:");
	await waitFor(() =>
		expect(modelsSpy).toHaveBeenCalledWith({
			q: 'model:"gpt\\_4\\%:"',
			limit: 25,
		}),
	);
});
