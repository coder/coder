import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ProxyContextValue } from "contexts/ProxyContext";
import { MockPrimaryWorkspaceProxy, MockUser } from "testHelpers/entities";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { NavbarView } from "./NavbarView";

const proxyContextValue: ProxyContextValue = {
	proxy: {
		preferredPathAppURL: "",
		preferredWildcardHostname: "",
		proxy: MockPrimaryWorkspaceProxy,
	},
	isLoading: false,
	isFetched: true,
	setProxy: jest.fn(),
	clearProxy: jest.fn(),
	refetchProxyLatencies: jest.fn(),
	proxyLatencies: {},
};

describe("NavbarView", () => {
	const noop = jest.fn();

	it("workspaces nav link has the correct href", async () => {
		renderWithAuth(
			<NavbarView
				proxyContextValue={proxyContextValue}
				user={MockUser}
				onSignOut={noop}
				canViewDeployment
				canViewOrganizations
				canViewHealth
				canViewAuditLog
			/>,
		);
		const workspacesLink =
			await screen.findByText<HTMLAnchorElement>(/workspaces/i);
		expect(workspacesLink.href).toContain("/workspaces");
	});

	it("templates nav link has the correct href", async () => {
		renderWithAuth(
			<NavbarView
				proxyContextValue={proxyContextValue}
				user={MockUser}
				onSignOut={noop}
				canViewDeployment
				canViewOrganizations
				canViewHealth
				canViewAuditLog
			/>,
		);
		const templatesLink =
			await screen.findByText<HTMLAnchorElement>(/templates/i);
		expect(templatesLink.href).toContain("/templates");
	});

	it("audit nav link has the correct href", async () => {
		renderWithAuth(
			<NavbarView
				proxyContextValue={proxyContextValue}
				user={MockUser}
				onSignOut={noop}
				canViewDeployment
				canViewOrganizations
				canViewHealth
				canViewAuditLog
			/>,
		);
		const deploymentMenu = await screen.findByText("Admin settings");
		await userEvent.click(deploymentMenu);
		const auditLink = await screen.findByText<HTMLAnchorElement>(/audit logs/i);
		expect(auditLink.href).toContain("/audit");
	});

	it("deployment nav link has the correct href", async () => {
		renderWithAuth(
			<NavbarView
				proxyContextValue={proxyContextValue}
				user={MockUser}
				onSignOut={noop}
				canViewDeployment
				canViewOrganizations
				canViewHealth
				canViewAuditLog
			/>,
		);
		const deploymentMenu = await screen.findByText("Admin settings");
		await userEvent.click(deploymentMenu);
		const deploymentSettingsLink =
			await screen.findByText<HTMLAnchorElement>(/deployment/i);
		expect(deploymentSettingsLink.href).toContain("/deployment/general");
	});
});
