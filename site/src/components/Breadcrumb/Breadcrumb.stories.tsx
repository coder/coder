import { MockOrganization } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	Breadcrumb,
	BreadcrumbEllipsis,
	BreadcrumbItem,
	BreadcrumbLink,
	BreadcrumbList,
	BreadcrumbPage,
	BreadcrumbSeparator,
} from "components/Breadcrumb/Breadcrumb";

const meta: Meta<typeof Breadcrumb> = {
	title: "components/Breadcrumb",
	component: Breadcrumb,
};

export default meta;
type Story = StoryObj<typeof Breadcrumb>;

export const Default: Story = {
	args: {
		children: (
			<Breadcrumb>
				<BreadcrumbList>
					<BreadcrumbItem>
						<BreadcrumbPage>Admin Settings</BreadcrumbPage>
					</BreadcrumbItem>
					<BreadcrumbSeparator />
					<BreadcrumbItem>
						<BreadcrumbEllipsis />
					</BreadcrumbItem>
					<BreadcrumbSeparator />
					<BreadcrumbItem>
						<BreadcrumbLink href="/organizations">Organizations</BreadcrumbLink>
					</BreadcrumbItem>
					<BreadcrumbSeparator />
					<BreadcrumbItem>
						<BreadcrumbPage className="text-content-primary">
							{MockOrganization.name}
						</BreadcrumbPage>
					</BreadcrumbItem>
				</BreadcrumbList>
			</Breadcrumb>
		),
	},
};
