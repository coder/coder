import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { useClientFilterMenu } from "./ClientFilter";

const mockClients = ["claude-code", "claude-code-router"];

const renderClientFilterMenu = (value: string) => {
	vi.spyOn(API, "getAIBridgeClients").mockImplementation(async ({ q, limit }) =>
		mockClients.filter((client) => client.startsWith(q ?? "")).slice(0, limit),
	);
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	const wrapper = ({ children }: PropsWithChildren) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
	const rendered = renderHook(
		() => useClientFilterMenu({ value, onChange: vi.fn(), enabled: true }),
		{ wrapper },
	);
	return { ...rendered, queryClient };
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe("useClientFilterMenu", () => {
	it("resolves the selected client when the search returns it", async () => {
		const { result, unmount, queryClient } =
			renderClientFilterMenu("claude-code");

		await waitFor(() => expect(result.current.isInitializing).toBe(false));
		expect(result.current.selectedOption).toMatchObject({
			label: "claude-code",
			value: "claude-code",
		});

		unmount();
		queryClient.clear();
	});

	it("does not select a client that only matches the value as a prefix", async () => {
		const { result, unmount, queryClient } = renderClientFilterMenu("claude");

		await waitFor(() => expect(result.current.isInitializing).toBe(false));
		expect(result.current.selectedOption).toBeNull();

		unmount();
		queryClient.clear();
	});
});
