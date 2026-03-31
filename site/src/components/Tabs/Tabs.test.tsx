import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { TabLink, Tabs, TabsList } from "./Tabs";

const renderTabs = (active = "overview") => {
	render(
		<MemoryRouter>
			<Tabs active={active}>
				<TabsList>
					<TabLink to="/overview" value="overview">
						Overview
					</TabLink>
					<TabLink to="/settings" value="settings">
						Settings
					</TabLink>
				</TabsList>
			</Tabs>
		</MemoryRouter>,
	);
};

describe("Tabs", () => {
	it("does not expose tablist semantics for link navigation", () => {
		renderTabs();

		expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
	});

	it("marks only the active tab link as the current page", () => {
		renderTabs("overview");

		expect(screen.getByRole("link", { name: "Overview" })).toHaveAttribute(
			"aria-current",
			"page",
		);
		expect(screen.getByRole("link", { name: "Settings" })).not.toHaveAttribute(
			"aria-current",
		);
	});
});
