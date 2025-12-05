import { render } from "testHelpers/renderHelpers";
import { screen } from "@testing-library/react";
import type * as TypesGen from "api/typesGenerated";
import { vi } from "vitest";
import { NavbarView } from "./NavbarView";

const mockUser: TypesGen.User = {
	id: "test-user",
	username: "testuser",
	email: "test@example.com",
	name: "Test User",
	created_at: "2023-01-01T00:00:00Z",
	status: "active",
	organization_ids: [],
	roles: [],
	avatar_url: "",
	last_seen_at: "2023-01-01T00:00:00Z",
	login_type: "password",
	theme_preference: "",
};

describe("NavbarView", () => {
	const defaultProps = {
		user: mockUser,
		supportLinks: [],
		onSignOut: vi.fn(),
		canViewDeployment: false,
		canViewOrganizations: false,
		canViewAuditLog: false,
		canViewConnectionLog: false,
		canViewHealth: false,
		canViewAIBridge: false,
	};

	it("should render logo with hover animation classes", async () => {
		render(<NavbarView {...defaultProps} />);

		const logoLink = await screen.findByRole("link", { name: /coder logo/i });
		expect(logoLink).toHaveClass(
			"inline-block",
			"transition-transform",
			"hover:animate-logo-jump",
		);
	});

	it("should render custom logo with hover animation classes", async () => {
		render(
			<NavbarView {...defaultProps} logo_url="https://example.com/logo.png" />,
		);

		const logoLink = await screen.findByRole("link", { name: /custom logo/i });
		expect(logoLink).toHaveClass(
			"inline-block",
			"transition-transform",
			"hover:animate-logo-jump",
		);
	});
});
