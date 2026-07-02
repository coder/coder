import { act, waitFor } from "@testing-library/react";
import { useSearchParams } from "react-router";
import { renderHookWithAuth } from "#/testHelpers/hooks";
import {
	type AgentSidebarFilters,
	getAgentSidebarFilters,
} from "./agentSidebarFilters";

const defaultFilters: AgentSidebarFilters = {
	archiveStatus: "active",
	groupBy: "date",
	prStatuses: [],
	chatStatuses: ["unread", "read"],
	sources: ["created_by_me"],
};

const archivedFilters: AgentSidebarFilters = {
	archiveStatus: "archived",
	groupBy: "chat_status",
	prStatuses: ["draft", "merged"],
	chatStatuses: ["unread"],
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
				"/agents?archived=archived&group_by=chat_status&pr_status=open,draft,closed&chat_status=unread&source=shared_with_me",
			expected: {
				archiveStatus: "archived",
				groupBy: "chat_status",
				prStatuses: ["draft", "open", "closed"],
				chatStatuses: ["unread"],
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
	])("$name", async ({ route, expected }) => {
		const { result } = await renderFilters(route);
		expect(result.current[0]).toEqual(expected);
	});

	it("omits default values when writing filters", async () => {
		const { result, getLocationSnapshot } = await renderFilters(
			"/agents?archived=archived&group_by=chat_status&pr_status=draft&chat_status=unread",
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
		expect(search.get("chat_status")).toBe("unread");
		expect(search.get("source")).toBe("created_by_me,shared_with_me");
	});
});
