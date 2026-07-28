import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { fn, userEvent, within } from "storybook/test";
import {
	MockPrimaryWorkspaceProxy,
	MockProxyLatencies,
	MockSupportLinks,
	MockUserMember,
	MockUserOwner,
	MockWorkspaceProxies,
} from "#/testHelpers/entities";
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
			latenciesLoaded: true,
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
		user: MockUserOwner,
		supportLinks: MockSupportLinks,
		onSignOut: fn(),
		isDefaultOpen: true,
		adminPermissions: {
			canViewDeployment: true,
			canViewOrganizations: true,
			canViewAISettings: true,
			canViewAuditLog: true,
			canViewConnectionLog: true,
			canViewAIBridge: true,
			canViewHealth: true,
		},
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
		user: MockUserMember,
		adminPermissions: {
			canViewAuditLog: true,
		},
	},
	play: openAdminSettings,
};

export const OrgAdmin: Story = {
	args: {
		user: MockUserMember,
		adminPermissions: {
			canViewAuditLog: true,
			canViewOrganizations: true,
		},
	},
	play: openAdminSettings,
};

export const Member: Story = {
	args: {
		user: MockUserMember,
		adminPermissions: {},
	},
};

export const ProxySettings: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const menuItem = await body.findByRole("menuitem", {
			name: /workspace proxy settings/i,
		});
		await user.click(menuItem);
	},
};

export const UserSettings: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
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

async function openAdminSettings({
	canvasElement,
}: {
	canvasElement: HTMLElement;
}) {
	const user = userEvent.setup();
	const body = within(canvasElement.ownerDocument.body);
	const menuItem = await body.findByRole("menuitem", {
		name: /admin settings/i,
	});
	await user.click(menuItem);
}
