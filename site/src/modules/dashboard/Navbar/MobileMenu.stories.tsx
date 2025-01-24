import type { Meta, StoryObj } from "@storybook/react";
import { fn, userEvent, within } from "@storybook/test";
import { PointerEventsCheckLevel } from "@testing-library/user-event";
import type { FC } from "react";
import { chromaticWithTablet } from "testHelpers/chromatic";
import {
	MockPrimaryWorkspaceProxy,
	MockProxyLatencies,
	MockSupportLinks,
	MockUser,
	MockUser2,
	MockWorkspaceProxies,
} from "testHelpers/entities";
import { MobileMenu } from "./MobileMenu";

const meta: Meta<typeof MobileMenu> = {
	title: "modules/dashboard/MobileMenu",
	parameters: {
		layout: "fullscreen",
		viewport: {
			defaultViewport: "iphone12",
		},
	},
	component: MobileMenu,
	args: {
		proxyContextValue: {
			proxy: {
				preferredPathAppURL: "",
				preferredWildcardHostname: "",
				proxy: MockPrimaryWorkspaceProxy,
			},
			isLoading: false,
			isFetched: true,
			setProxy: fn(),
			clearProxy: fn(),
			refetchProxyLatencies: fn(),
			proxyLatencies: MockProxyLatencies,
			proxies: MockWorkspaceProxies,
		},
		user: MockUser,
		supportLinks: MockSupportLinks,
		onSignOut: fn(),
		isDefaultOpen: true,
		canViewAuditLog: true,
		canViewDeployment: true,
		canViewHealth: true,
		canViewOrganizations: true,
	},
	decorators: [withNavbarMock],
};

export default meta;
type Story = StoryObj<typeof MobileMenu>;

export const Closed: Story = {
	args: {
		isDefaultOpen: false,
	},
};

export const Admin: Story = {
	play: openAdminSettings,
};

export const Auditor: Story = {
	args: {
		user: MockUser2,
		canViewAuditLog: true,
		canViewDeployment: false,
		canViewHealth: false,
		canViewOrganizations: false,
	},
	play: openAdminSettings,
};

export const OrgAdmin: Story = {
	args: {
		user: MockUser2,
		canViewAuditLog: true,
		canViewDeployment: false,
		canViewHealth: false,
		canViewOrganizations: true,
	},
	play: openAdminSettings,
};

export const Member: Story = {
	args: {
		user: MockUser2,
		canViewAuditLog: false,
		canViewDeployment: false,
		canViewHealth: false,
		canViewOrganizations: false,
	},
};

export const ProxySettings: Story = {
	play: async ({ canvasElement }) => {
		const user = setupUser();
		const body = within(canvasElement.ownerDocument.body);
		const menuItem = await body.findByRole("menuitem", {
			name: /workspace proxy settings/i,
		});
		await user.click(menuItem);
	},
};

export const UserSettings: Story = {
	play: async ({ canvasElement }) => {
		const user = setupUser();
		const body = within(canvasElement.ownerDocument.body);
		const menuItem = await body.findByRole("menuitem", {
			name: /user settings/i,
		});
		await user.click(menuItem);
	},
};

function withNavbarMock(Story: FC) {
	return (
		<div className="h-[72px] border-0 border-b border-solid px-6 flex items-center justify-end">
			<Story />
		</div>
	);
}

function setupUser() {
	// It seems the dropdown component is disabling pointer events, which is
	// causing Testing Library to throw an error. As a workaround, we can
	// disable the pointer events check.
	return userEvent.setup({
		pointerEventsCheck: PointerEventsCheckLevel.Never,
	});
}

async function openAdminSettings({
	canvasElement,
}: { canvasElement: HTMLElement }) {
	const user = setupUser();
	const body = within(canvasElement.ownerDocument.body);
	const menuItem = await body.findByRole("menuitem", {
		name: /admin settings/i,
	});
	await user.click(menuItem);
}
