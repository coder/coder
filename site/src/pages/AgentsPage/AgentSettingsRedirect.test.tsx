import { render, screen } from "@testing-library/react";
import type { FC } from "react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { describe, expect, it } from "vitest";
import AgentSettingsRedirect from "./AgentSettingsRedirect";

const LocationProbe: FC = () => {
	const location = useLocation();
	return (
		<div data-testid="location">{`${location.pathname}${location.search}`}</div>
	);
};

const renderAt = (path: string) => {
	render(
		<MemoryRouter initialEntries={[path]}>
			<Routes>
				<Route path="/agents/settings" element={<AgentSettingsRedirect />} />
				<Route
					path="/agents/settings/:section"
					element={<AgentSettingsRedirect />}
				/>
				<Route path="*" element={<LocationProbe />} />
			</Routes>
		</MemoryRouter>,
	);
};

describe("AgentSettingsRedirect", () => {
	it.each([
		["/agents/settings", "/agents?settings=general"],
		["/agents/settings/api-keys", "/agents?settings=api-keys"],
		["/agents/settings/unknown", "/agents?settings=general"],
		[
			"/agents/settings/compaction?archived=archived",
			"/agents?archived=archived&settings=compaction",
		],
		["/agents/settings/models", "/ai/settings/models"],
		["/agents/settings/experiments", "/ai/settings/coder-agents"],
	])("redirects %s to %s", (from, to) => {
		renderAt(from);

		expect(screen.getByTestId("location").textContent).toBe(to);
	});
});
