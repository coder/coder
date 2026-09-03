import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockChatModel } from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import {
	organizationAddModelPath,
	organizationModelsPath,
	selectModelOrganization,
	useAccessibleModelOrganizations,
} from "./organizationModels";

const createQueryWrapper = () => {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	const wrapper = ({ children }: PropsWithChildren) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
	return { queryClient, wrapper };
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe("selectModelOrganization", () => {
	const organizations = [MockDefaultOrganization, MockOrganization2];

	it("falls back only when no organization is requested", () => {
		expect(selectModelOrganization(organizations, null)).toEqual({
			organization: MockDefaultOrganization,
			requestedOrganizationDenied: false,
		});
		expect(
			selectModelOrganization(
				organizations.map((organization) => ({
					...organization,
					is_default: false,
				})),
				null,
			).organization?.id,
		).toBe(MockDefaultOrganization.id);
		expect(selectModelOrganization([], null)).toEqual({
			organization: undefined,
			requestedOrganizationDenied: false,
		});
	});

	it("uses exactly the requested accessible organization", () => {
		expect(
			selectModelOrganization(organizations, MockOrganization2.name),
		).toEqual({
			organization: MockOrganization2,
			requestedOrganizationDenied: false,
		});
	});

	it("marks a requested missing organization as denied", () => {
		expect(selectModelOrganization(organizations, "missing")).toEqual({
			organization: MockDefaultOrganization,
			requestedOrganizationDenied: true,
		});
	});

	it("builds an organization-scoped models path", () => {
		expect(organizationModelsPath(MockOrganization2)).toBe(
			`/ai/settings/models?org=${MockOrganization2.name}`,
		);
	});

	it("preserves auxiliary parameters in organization model paths", () => {
		const params = new URLSearchParams({
			provider: "openai",
			duplicate: "model-id",
		});

		expect(organizationAddModelPath(MockOrganization2, params)).toBe(
			`/ai/settings/models/add?provider=openai&duplicate=model-id&org=${MockOrganization2.name}`,
		);
	});
});

describe("useAccessibleModelOrganizations", () => {
	it("excludes successful empty collection reads without model permissions", async () => {
		vi.spyOn(API, "checkAuthorization").mockResolvedValue({});
		const getChatModels = vi
			.spyOn(API.experimental, "getChatModels")
			.mockImplementation(async (_organizationId) => ({
				models: [],
				providers: [],
				unsupported_providers: [],
			}));
		const { queryClient, wrapper } = createQueryWrapper();
		const { result, unmount } = renderHook(
			() =>
				useAccessibleModelOrganizations([
					MockDefaultOrganization,
					MockOrganization2,
				]),
			{ wrapper },
		);

		await waitFor(() => expect(result.current.isLoading).toBe(false));
		expect(result.current.organizations).toEqual([]);
		expect(getChatModels).toHaveBeenCalledTimes(2);

		unmount();
		queryClient.clear();
	});

	it("includes organization-level model access without readable rows", async () => {
		vi.spyOn(API, "checkAuthorization").mockResolvedValue({
			[`${MockDefaultOrganization.id}.viewChatModelConfigs`]: true,
		});
		vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
			models: [],
			providers: [],
			unsupported_providers: [],
		});
		const { queryClient, wrapper } = createQueryWrapper();
		const { result, unmount } = renderHook(
			() =>
				useAccessibleModelOrganizations([
					MockDefaultOrganization,
					MockOrganization2,
				]),
			{ wrapper },
		);

		await waitFor(() => expect(result.current.isLoading).toBe(false));
		expect(result.current.organizations).toEqual([MockDefaultOrganization]);

		unmount();
		queryClient.clear();
	});

	it("includes ACL-filtered readable model rows without collection access", async () => {
		vi.spyOn(API, "checkAuthorization").mockResolvedValue({});
		vi.spyOn(API.experimental, "getChatModels").mockImplementation(
			async (organizationId) => ({
				models:
					organizationId === MockDefaultOrganization.id
						? [
								{
									...MockChatModel,
									organization_id: organizationId,
								},
							]
						: [],
				providers: [],
				unsupported_providers: [],
			}),
		);
		const { queryClient, wrapper } = createQueryWrapper();
		const { result, unmount } = renderHook(
			() =>
				useAccessibleModelOrganizations([
					MockDefaultOrganization,
					MockOrganization2,
				]),
			{ wrapper },
		);

		await waitFor(() => expect(result.current.isLoading).toBe(false));
		expect(result.current.organizations).toEqual([MockDefaultOrganization]);

		unmount();
		queryClient.clear();
	});

	it("keeps accessible organization data when another request fails", async () => {
		vi.spyOn(API, "checkAuthorization").mockResolvedValue({
			[`${MockDefaultOrganization.id}.viewChatModelConfigs`]: true,
		});
		vi.spyOn(API.experimental, "getChatModels").mockImplementation(
			async (organizationId) => {
				if (organizationId === MockOrganization2.id) {
					throw new Error("Failed to load organization");
				}
				return { models: [], providers: [], unsupported_providers: [] };
			},
		);
		const { queryClient, wrapper } = createQueryWrapper();
		const { result, unmount } = renderHook(
			() =>
				useAccessibleModelOrganizations([
					MockDefaultOrganization,
					MockOrganization2,
				]),
			{ wrapper },
		);

		await waitFor(() => expect(result.current.isLoading).toBe(false));
		expect(result.current.organizations).toEqual([MockDefaultOrganization]);
		expect(result.current.error).toBeNull();
		expect(result.current.partialError).toEqual(
			new Error("Failed to load organization"),
		);

		unmount();
		queryClient.clear();
	});
});
