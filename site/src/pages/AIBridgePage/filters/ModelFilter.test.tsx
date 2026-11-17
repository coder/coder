import { renderHook, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { createTestQueryClient, render } from "#/testHelpers/renderHelpers";
import { ModelFilter, useModelFilterMenu } from "./ModelFilter";

afterEach(() => {
	vi.restoreAllMocks();
});

const ModelFilterHarness: React.FC = () => {
	const menu = useModelFilterMenu({
		value: undefined,
		onChange: vi.fn(),
		enabled: true,
	});
	return <ModelFilter menu={menu} />;
};

it("selects the filter value when the search returns another model first", async () => {
	vi.spyOn(API, "getAIBridgeModels").mockResolvedValue([
		"us.anthropic.claude-3-5-sonnet-20241022-v2:0",
		"claude-3-5-sonnet",
	]);
	const queryClient = createTestQueryClient();
	const { result } = renderHook(
		() =>
			useModelFilterMenu({
				value: "claude-3-5-sonnet",
				onChange: vi.fn(),
				enabled: true,
			}),
		{
			wrapper: ({ children }: React.PropsWithChildren) => (
				<QueryClientProvider client={queryClient}>
					{children}
				</QueryClientProvider>
			),
		},
	);
	await waitFor(() =>
		expect(result.current.selectedOption).toMatchObject({
			label: "claude-3-5-sonnet",
			value: "claude-3-5-sonnet",
		}),
	);
});

it("sends the typed text as the literal model search", async () => {
	const user = userEvent.setup();
	const modelsSpy = vi.spyOn(API, "getAIBridgeModels").mockResolvedValue([]);
	render(<ModelFilterHarness />);
	await user.click(screen.getByRole("button", { name: "Select model" }));
	await user.type(await screen.findByRole("combobox"), "gpt_4%:");
	await waitFor(() =>
		expect(modelsSpy).toHaveBeenCalledWith({ model: "gpt_4%:", limit: 25 }),
	);
});
