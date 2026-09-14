import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import {
	LinkTabs,
	LinkTabsList,
	TabLink,
	Tabs,
	TabsContent,
	TabsList,
	TabsTrigger,
} from "./Tabs";

const renderLinkTabs = (active = "overview") => {
	render(
		<MemoryRouter>
			<LinkTabs active={active}>
				<LinkTabsList>
					<TabLink to="/overview" value="overview">
						Overview
					</TabLink>
					<TabLink to="/settings" value="settings">
						Settings
					</TabLink>
				</LinkTabsList>
			</LinkTabs>
		</MemoryRouter>,
	);
};

describe("LinkTabs", () => {
	it("does not expose tablist semantics for link navigation", () => {
		renderLinkTabs();

		expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
	});

	it("marks only the active tab link as the current page", () => {
		renderLinkTabs("overview");

		expect(screen.getByRole("link", { name: "Overview" })).toHaveAttribute(
			"aria-current",
			"page",
		);
		expect(screen.getByRole("link", { name: "Settings" })).not.toHaveAttribute(
			"aria-current",
		);
	});
});

describe("Tabs (Radix)", () => {
	it("exposes tablist semantics for keyboard navigation", () => {
		render(
			<Tabs defaultValue="a">
				<TabsList variant="insideBox" aria-label="Example">
					<TabsTrigger value="a">Alpha</TabsTrigger>
					<TabsTrigger value="b">Beta</TabsTrigger>
				</TabsList>
				<TabsContent value="a">A</TabsContent>
				<TabsContent value="b">B</TabsContent>
			</Tabs>,
		);

		expect(
			screen.getByRole("tablist", { name: "Example" }),
		).toBeInTheDocument();
		expect(screen.getByRole("tab", { name: "Alpha" })).toHaveAttribute(
			"data-state",
			"active",
		);
	});
});
