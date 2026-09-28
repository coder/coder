import { act, waitFor } from "@testing-library/react";
import { useSearchParams } from "react-router";
import { renderHookWithAuth } from "#/testHelpers/hooks";
import {
	AGENT_CHAT_STATUS_ORDER,
	type AgentSidebarFilters,
	getAgentSidebarFilters,
} from "./agentSidebarFilters";

const defaultFilters: AgentSidebarFilters = {
	archiveStatus: "active",
	groupBy: "date",
	prStatuses: [],
	chatStatuses: AGENT_CHAT_STATUS_ORDER,
	unread: false,
	sources: ["created_by_me"],
};

const archivedFilters: AgentSidebarFilters = {
	archiveStatus: "archived",
	groupBy: "chat_status",
	prStatuses: ["draft", "merged"],
	chatStatuses: ["running"],
	unread: false,
	sources: ["created_by_me", "shared_with_me"],
};

const renderFilters = (route = "/agents") => {
	return renderHookWithAuth(
		() => {
			const [searchParams, setSearchParams] = useSearchParams();
			return getAgentSidebarFilters(searchParams, setSearchParams);
		},
		{
			routingOptions: { path: "/agents", route },
		},
	);
};

describe(getAgentSidebarFilters.name, () => {
	it.each<{
		name: string;
		route: string;
		expected: AgentSidebarFilters;
	}>([
		{
			name: "returns defaults for /agents",
			route: "/agents",
			expected: defaultFilters,
		},
		{
			name: "parses archived, group_by, pr_status, chat_status, and source",
			route:
				"/agents?archived=archived&group_by=chat_status&pr_status=open,draft,closed&chat_status=running&source=shared_with_me",
			expected: {
				archiveStatus: "archived",
				groupBy: "chat_status",
				prStatuses: ["draft", "open", "closed"],
				chatStatuses: ["running"],
				unread: false,
				sources: ["shared_with_me"],
			},
		},
		{
			name: "drops invalid pr_status values and canonicalizes order",
			route: "/agents?pr_status=merged,bogus,draft",
			expected: {
				...defaultFilters,
				prStatuses: ["draft", "merged"],
			},
		},
		{
			name: "keeps the none pull request status",
			route: "/agents?pr_status=none,bogus,draft",
			expected: {
				...defaultFilters,
				prStatuses: ["draft", "none"],
			},
		},
	])("$name", async ({ route, expected }) => {
		const { result } = await renderFilters(route);
		expect(result.current[0]).toEqual(expected);
	});

	it("omits default values when writing filters", async () => {
		const { result, getLocationSnapshot } = await renderFilters(
			"/agents?archived=archived&group_by=chat_status&pr_status=draft&chat_status=running",
		);

		act(() => {
			result.current[1](defaultFilters);
		});
		await waitFor(() => expect(result.current[0]).toEqual(defaultFilters));

		const { search } = getLocationSnapshot();
		expect(search.get("archived")).toEqual(null);
		expect(search.get("group_by")).toEqual(null);
		expect(search.get("pr_status")).toEqual(null);
		expect(search.get("chat_status")).toEqual(null);
		expect(search.get("source")).toEqual(null);
	});

	it("writes archived status filter", async () => {
		const { result, getLocationSnapshot } = await renderFilters();

		act(() => {
			result.current[1]({ ...defaultFilters, archiveStatus: "archived" });
		});
		await waitFor(() =>
			expect(result.current[0]).toMatchObject({
				archiveStatus: "archived",
			}),
		);

		const { search } = getLocationSnapshot();
		expect(search.get("archived")).toEqual("archived");
		expect(search.get("chat_status")).toEqual(null);
	});

	it("preserves unrelated search params when writing filters", async () => {
		const { result, getLocationSnapshot } = await renderFilters(
			"/agents?tab=settings&foo=bar&archived=archived",
		);

		act(() => {
			result.current[1](archivedFilters);
		});
		await waitFor(() => expect(result.current[0]).toEqual(archivedFilters));

		const { search } = getLocationSnapshot();
		expect(search.get("tab")).toBe("settings");
		expect(search.get("foo")).toBe("bar");
		expect(search.get("archived")).toBe("archived");
		expect(search.get("group_by")).toBe("chat_status");
		expect(search.get("pr_status")).toBe("draft,merged");
		expect(search.get("chat_status")).toBe("running");
		expect(search.get("source")).toBe("created_by_me,shared_with_me");
	});

	it("writes a partial status selection in canonical order", async () => {
		const { result, getLocationSnapshot } = await renderFilters();

		act(() => {
			result.current[1]({
				...defaultFilters,
				chatStatuses: ["running", "error"],
			});
		});
		await waitFor(() =>
			expect(result.current[0].chatStatuses).toEqual(["error", "running"]),
		);

		expect(getLocationSnapshot().search.get("chat_status")).toBe(
			"error,running",
		);
	});

	it("writes the unread filter without treating it as a chat status", async () => {
		const { result, getLocationSnapshot } = await renderFilters();

		act(() => {
			result.current[1]({ ...defaultFilters, unread: true });
		});
		await waitFor(() => expect(result.current[0].unread).toBe(true));

		const { search } = getLocationSnapshot();
		expect(search.get("unread")).toBe("true");
		expect(search.get("chat_status")).toBe(null);
	});

	it("reads the unread filter", async () => {
		const { result } = await renderFilters("/agents?unread=true");
		expect(result.current[0].unread).toBe(true);
		expect(result.current[0].chatStatuses).toEqual(AGENT_CHAT_STATUS_ORDER);
	});

	it("ignores interrupting and other values that are not filter options", async () => {
		const { result } = await renderFilters(
			"/agents?chat_status=working,interrupting,done",
		);
		expect(result.current[0].chatStatuses).toEqual(AGENT_CHAT_STATUS_ORDER);
	});
});
