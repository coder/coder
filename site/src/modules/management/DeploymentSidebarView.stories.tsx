import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import { SidebarContext } from "#/components/Sidebar/SidebarContext";
import {
	MockBuildInfo,
	MockNoPermissions,
	MockPermissions,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { DeploymentSidebarView } from "./DeploymentSidebarView";

const meta: Meta<typeof DeploymentSidebarView> = {
	title: "modules/management/DeploymentSidebarView",
	component: DeploymentSidebarView,
	decorators: [withDashboardProvider],
	parameters: { showOrganizations: true },
	args: {
		permissions: MockPermissions,
		experiments: [],
		buildInfo: MockBuildInfo,
		activeSection: "general",
	},
};

export default meta;
type Story = StoryObj<typeof DeploymentSidebarView>;

export const GeneralOpen: Story = {};

export const InfrastructureOpen: Story = {
	args: {
		activeSection: "infrastructure",
	},
};

export const AuthenticationOpen: Story = {
	args: {
		activeSection: "authentication",
	},
};

export const Collapsed: Story = {
	decorators: [
		(Story) => (
			<SidebarContext.Provider
				value={{
					collapsed: true,
					expand: () => {},
					collapse: () => {},
					toggle: () => {},
				}}
			>
				<Story />
			</SidebarContext.Provider>
		),
	],
};

export const NoViewUsers: Story = {
	args: {
		permissions: {
			...MockPermissions,
			viewAllUsers: false,
		},
	},
};

export const NoAuditLog: Story = {
	args: {
		permissions: {
			...MockPermissions,
			viewAnyAuditLog: false,
		},
	},
};

export const NoLicenses: Story = {
	args: {
		permissions: {
			...MockPermissions,
			viewAllLicenses: false,
		},
	},
};

export const NoDeploymentValues: Story = {
	args: {
		permissions: {
			...MockPermissions,
			viewDeploymentConfig: false,
			editDeploymentConfig: false,
		},
	},
};

export const NoPermissions: Story = {
	args: {
		permissions: MockNoPermissions,
	},
};

// Renders inside a real CollapsibleSidebar at a narrow viewport. Even
// though the persisted preference is "expanded", the sidebar must
// auto-collapse to the icon rail below the lg breakpoint.
export const NarrowViewportAutoCollapse: Story = {
	decorators: [
		(Story) => {
			localStorage.setItem("story-deployment-narrow", "expanded");
			return (
				<CollapsibleSidebar storageKey="story-deployment-narrow">
					<Story />
				</CollapsibleSidebar>
			);
		},
	],
	parameters: {
		viewport: { defaultViewport: "iphone12" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Collapsed accordions render icon-only triggers, so the labeled
		// "General" trigger must not exist at a narrow viewport.
		await waitFor(() => {
			expect(
				canvas.queryByRole("button", { name: /general/i }),
			).not.toBeInTheDocument();
		});
	},
};
