import { renderHook, waitFor } from "@testing-library/react";
import type { FC, PropsWithChildren } from "react";
import {
	focusManager,
	type QueryClient,
	QueryClientProvider,
	useQuery,
} from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Experiment, User } from "#/api/typesGenerated";
import type { RuntimeHtmlMetadata } from "#/hooks/useEmbeddedMetadata";
import { MockUserMember, MockUserOwner } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { experiments } from "./experiments";

const embeddedExperiments: Experiment[] = ["example"];
const fetchedExperiments: Experiment[] = ["mcp-tool-search"];

// Metadata as embedded in a page rendered for user.
const pageMetadata = (
	user: User,
): Pick<RuntimeHtmlMetadata, "user" | "experiments"> => ({
	user: { available: true, value: user },
	experiments: { available: true, value: embeddedExperiments },
});

// Embedded metadata is as old as the page, so tests move the clock
// relative to page load.
const setTimeSincePageLoad = (ms: number) => {
	vi.setSystemTime(performance.timeOrigin + ms);
};

// React Query handles focus after a microtask; waiting for the next
// macrotask lets any refetch it triggers start.
const focusWindow = async () => {
	focusManager.setFocused(false);
	focusManager.setFocused(true);
	await new Promise((resolve) => setTimeout(resolve, 0));
};

const renderExperiments = (
	queryClient: QueryClient,
	userId: string,
	metadata: Pick<RuntimeHtmlMetadata, "user" | "experiments">,
) => {
	const wrapper: FC<PropsWithChildren> = ({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
	return renderHook(() => useQuery(experiments(userId, metadata)), {
		wrapper,
	});
};

describe("experiments", () => {
	beforeEach(() => {
		// Fake only Date so React Query's staleness follows the test clock
		// while its timers keep running.
		vi.useFakeTimers({ toFake: ["Date"] });
		vi.spyOn(API, "getExperiments").mockResolvedValue(fetchedExperiments);
	});

	afterEach(() => {
		focusManager.setFocused(undefined);
		vi.useRealTimers();
		vi.restoreAllMocks();
	});

	it("seeds from embedded metadata only for the user it was rendered for", async () => {
		setTimeSincePageLoad(1_000);
		const queryClient = createTestQueryClient();
		const metadata = pageMetadata(MockUserOwner);

		const owner = renderExperiments(queryClient, MockUserOwner.id, metadata);
		expect(owner.result.current.data).toEqual(embeddedExperiments);
		expect(API.getExperiments).not.toHaveBeenCalled();

		// Another user in the same cache must not see the owner's list.
		const member = renderExperiments(queryClient, MockUserMember.id, metadata);
		await waitFor(() =>
			expect(member.result.current.data).toEqual(fetchedExperiments),
		);
		expect(API.getExperiments).toHaveBeenCalledTimes(1);
	});

	it("treats reused metadata as stale from page load after a remount", async () => {
		setTimeSincePageLoad(1_000);
		const queryClient = createTestQueryClient();
		const metadata = pageMetadata(MockUserOwner);

		const first = renderExperiments(queryClient, MockUserOwner.id, metadata);
		expect(API.getExperiments).not.toHaveBeenCalled();
		first.unmount();
		queryClient.removeQueries();

		// The page is now older than the stale time, so the metadata that
		// seeds the new query is stale too.
		setTimeSincePageLoad(61_000);
		const second = renderExperiments(queryClient, MockUserOwner.id, metadata);
		await waitFor(() =>
			expect(second.result.current.data).toEqual(fetchedExperiments),
		);
		expect(API.getExperiments).toHaveBeenCalledTimes(1);
	});

	it("refetches on window focus once stale", async () => {
		setTimeSincePageLoad(1_000);
		const queryClient = createTestQueryClient();
		const { result } = renderExperiments(
			queryClient,
			MockUserOwner.id,
			pageMetadata(MockUserOwner),
		);

		setTimeSincePageLoad(30_000);
		await focusWindow();
		expect(API.getExperiments).not.toHaveBeenCalled();

		setTimeSincePageLoad(61_000);
		await focusWindow();
		expect(API.getExperiments).toHaveBeenCalledTimes(1);
		await waitFor(() =>
			expect(result.current.data).toEqual(fetchedExperiments),
		);
	});
});
