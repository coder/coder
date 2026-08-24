import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { OrganizationChatModelsResponse } from "#/api/typesGenerated";
import { MockChatModel } from "#/testHelpers/chatModels";
import { useOrganizationChatModels } from "./useOrganizationChatModels";

const createQueryWrapper = (staleTime = 0) => {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false, staleTime } },
	});
	const wrapper = ({ children }: PropsWithChildren) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
	return { queryClient, wrapper };
};

const modelsResponse = (
	organizationId: string,
): OrganizationChatModelsResponse => ({
	models: [
		{
			...MockChatModel,
			id: `model-${organizationId}`,
			organization_id: organizationId,
		},
	],
	providers: [],
});

const apiError = (status: number, message: string) => ({
	isAxiosError: true,
	status,
	response: { status, data: { message } },
});

beforeEach(() => {
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
	);
});

afterEach(() => {
	vi.restoreAllMocks();
});

describe("useOrganizationChatModels", () => {
	it("keeps each organization's models and reuses fresh cache", async () => {
		const getChatModels = vi
			.spyOn(API.experimental, "getChatModels")
			.mockImplementation(async (organizationId) =>
				modelsResponse(organizationId),
			);
		const { queryClient, wrapper } = createQueryWrapper(
			Number.POSITIVE_INFINITY,
		);

		const first = renderHook(
			() => useOrganizationChatModels(["organization-1", "organization-2"]),
			{ wrapper },
		);
		await waitFor(() => expect(first.result.current.isLoading).toBe(false));

		expect(first.result.current.models).toEqual([
			...modelsResponse("organization-1").models,
			...modelsResponse("organization-2").models,
		]);
		expect(getChatModels).toHaveBeenCalledTimes(2);
		expect(getChatModels).toHaveBeenCalledWith("organization-1");
		expect(getChatModels).toHaveBeenCalledWith("organization-2");

		first.rerender();
		expect(getChatModels).toHaveBeenCalledTimes(2);
		first.unmount();

		const second = renderHook(
			() => useOrganizationChatModels(["organization-1", "organization-2"]),
			{ wrapper },
		);

		expect(second.result.current.isLoading).toBe(false);
		expect(second.result.current.models).toHaveLength(2);
		expect(getChatModels).toHaveBeenCalledTimes(2);

		second.unmount();
		queryClient.clear();
	});

	it("stays loading until every organization request settles", async () => {
		let resolveSecond: (value: OrganizationChatModelsResponse) => void =
			() => {};
		const secondResponse = new Promise<OrganizationChatModelsResponse>(
			(resolve) => {
				resolveSecond = resolve;
			},
		);
		vi.spyOn(API.experimental, "getChatModels").mockImplementation(
			async (organizationId) =>
				organizationId === "organization-1"
					? modelsResponse("organization-1")
					: secondResponse,
		);
		const { queryClient, wrapper } = createQueryWrapper();

		const { result, unmount } = renderHook(
			() => useOrganizationChatModels(["organization-1", "organization-2"]),
			{ wrapper },
		);

		await waitFor(() =>
			expect(result.current.models).toEqual(
				modelsResponse("organization-1").models,
			),
		);
		expect(result.current.isLoading).toBe(true);

		resolveSecond(modelsResponse("organization-2"));
		await waitFor(() => expect(result.current.isLoading).toBe(false));
		expect(result.current.models).toHaveLength(2);

		unmount();
		queryClient.clear();
	});

	it("loads ACL-readable models without organization model permission", async () => {
		vi.mocked(API.checkAuthorization).mockImplementation(async ({ checks }) =>
			Object.fromEntries(Object.keys(checks).map((key) => [key, false])),
		);
		const getChatModels = vi
			.spyOn(API.experimental, "getChatModels")
			.mockImplementation(async (organizationId) =>
				modelsResponse(organizationId),
			);
		const { queryClient, wrapper } = createQueryWrapper();

		const { result, unmount } = renderHook(
			() => useOrganizationChatModels(["organization-1"]),
			{ wrapper },
		);

		await waitFor(() => expect(result.current.isLoading).toBe(false));
		expect(result.current.models).toEqual(
			modelsResponse("organization-1").models,
		);
		expect(result.current.error).toBeNull();
		expect(result.current.partialError).toBeNull();
		expect(getChatModels).toHaveBeenCalledOnce();
		expect(getChatModels).toHaveBeenCalledWith("organization-1");
		expect(API.checkAuthorization).not.toHaveBeenCalled();

		unmount();
		queryClient.clear();
	});

	it("reports a partial failure while keeping successful models", async () => {
		vi.spyOn(API.experimental, "getChatModels").mockImplementation(
			async (organizationId) => {
				if (organizationId === "organization-2") {
					throw apiError(500, "Server error");
				}
				return modelsResponse(organizationId);
			},
		);
		const { queryClient, wrapper } = createQueryWrapper();

		const { result, unmount } = renderHook(
			() => useOrganizationChatModels(["organization-1", "organization-2"]),
			{ wrapper },
		);

		await waitFor(() => expect(result.current.isLoading).toBe(false));
		expect(result.current.models).toEqual(
			modelsResponse("organization-1").models,
		);
		expect(result.current.error).toBeNull();
		expect(result.current.partialError).not.toBeNull();

		unmount();
		queryClient.clear();
	});

	it("reports a blocking error when no organization returns data", async () => {
		vi.spyOn(API.experimental, "getChatModels").mockImplementation(async () => {
			throw apiError(500, "Server error");
		});
		const { queryClient, wrapper } = createQueryWrapper();

		const { result, unmount } = renderHook(
			() => useOrganizationChatModels(["organization-1", "organization-2"]),
			{ wrapper },
		);

		await waitFor(() => expect(result.current.isLoading).toBe(false));
		expect(result.current.models).toEqual([]);
		expect(result.current.error).not.toBeNull();
		expect(result.current.partialError).toBeNull();

		unmount();
		queryClient.clear();
	});
});
