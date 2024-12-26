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
				docsHref="https://docs.coder.com"
				proxyContextValue={proxyContextValue}
				user={MockUser}
				onSignOut={noop}
				canViewDeployment
				canViewOrganizations
				canViewAllUsers
				canViewHealth
				canViewAuditLog
			/>,
		);
		const workspacesLink = await screen.findByText("Workspaces");
		expect((workspacesLink as HTMLAnchorElement).href).toContain("/workspaces");
	});

	it("templates nav link has the correct href", async () => {
		renderWithAuth(
			<NavbarView
				docsHref="https://docs.coder.com"
				proxyContextValue={proxyContextValue}
				user={MockUser}
				onSignOut={noop}
				canViewDeployment
				canViewOrganizations
				canViewAllUsers
				canViewHealth
				canViewAuditLog
			/>,
		);
		const templatesLink = await screen.findByText("Templates");
		expect((templatesLink as HTMLAnchorElement).href).toContain("/templates");
	});

	it("audit nav link has the correct href", async () => {
		renderWithAuth(
			<NavbarView
				docsHref="https://docs.coder.com"
				proxyContextValue={proxyContextValue}
				user={MockUser}
				onSignOut={noop}
				canViewDeployment
				canViewOrganizations
				canViewAllUsers
				canViewHealth
				canViewAuditLog
			/>,
		);
		const deploymentMenu = await screen.findByText("Admin settings");
		await userEvent.click(deploymentMenu);
		const auditLink = await screen.findByText("Audit Logs");
		expect((auditLink as HTMLAnchorElement).href).toContain("/audit");
	});

	it("deployment nav link has the correct href", async () => {
		renderWithAuth(
			<NavbarView
				docsHref="https://docs.coder.com"
				proxyContextValue={proxyContextValue}
				user={MockUser}
				onSignOut={noop}
				canViewDeployment
				canViewOrganizations
				canViewAllUsers
				canViewHealth
				canViewAuditLog
			/>,
		);
		const deploymentMenu = await screen.findByText("Admin settings");
		await userEvent.click(deploymentMenu);
		const deploymentSettingsLink = await screen.findByText("Deployment");
		expect((deploymentSettingsLink as HTMLAnchorElement).href).toContain(
			"/deployment/general",
		);
	});
});
