import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { afterEach, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { useModelFilterMenu } from "./ModelFilter";

afterEach(() => {
	vi.restoreAllMocks();
});

const bedrockModel = "us.anthropic.claude-3-5-sonnet-20241022-v2:0";

function renderMenu(value: string | undefined) {
	const queryClient = createTestQueryClient();
	return renderHook(
		() => useModelFilterMenu({ value, onChange: vi.fn(), enabled: true }),
		{
			wrapper: ({ children }: PropsWithChildren) => (
				<QueryClientProvider client={queryClient}>
					{children}
				</QueryClientProvider>
			),
		},
	);
}

it("searches for the selected model as a literal", async () => {
	const modelsSpy = vi
		.spyOn(API, "getAIBridgeModels")
		.mockResolvedValue([bedrockModel]);
	const { result } = renderMenu(bedrockModel);
	await waitFor(() =>
		expect(result.current.selectedOption).toMatchObject({
			value: bedrockModel,
		}),
	);
	expect(modelsSpy).toHaveBeenCalledWith({
		q: `model:"${bedrockModel}"`,
		limit: 1,
	});
});

it("searches typed text as a literal model prefix", async () => {
	const modelsSpy = vi
		.spyOn(API, "getAIBridgeModels")
		.mockResolvedValue([bedrockModel]);
	const { result } = renderMenu(undefined);
	await waitFor(() =>
		expect(modelsSpy).toHaveBeenCalledWith({ q: 'model:""', limit: 25 }),
	);
	act(() => result.current.setQuery("us.anthropic:"));
	await waitFor(() =>
		expect(modelsSpy).toHaveBeenCalledWith({
			q: 'model:"us.anthropic:"',
			limit: 25,
		}),
	);
});
