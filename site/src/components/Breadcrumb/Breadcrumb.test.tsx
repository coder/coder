import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { BreadcrumbLink } from "./Breadcrumb";

describe(BreadcrumbLink.name, () => {
	it("does not mark a link to an ancestor page as the current page", () => {
		render(
			<MemoryRouter initialEntries={["/templates/docker/settings"]}>
				<BreadcrumbLink to="/templates/docker">docker</BreadcrumbLink>
			</MemoryRouter>,
		);

		expect(screen.getByRole("link", { name: "docker" })).not.toHaveAttribute(
			"aria-current",
		);
	});
});
