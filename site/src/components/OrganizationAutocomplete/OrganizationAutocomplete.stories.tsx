import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { MockOrganization, MockOrganization2 } from "#/testHelpers/entities";
import { OrganizationAutocomplete } from "./OrganizationAutocomplete";

const meta: Meta<typeof OrganizationAutocomplete> = {
	title: "components/OrganizationAutocomplete",
	component: OrganizationAutocomplete,
	args: {
		onChange: fn(),
		options: [MockOrganization, MockOrganization2],
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationAutocomplete>;

export const ManyOrgs: Story = {
	args: {
		value: null,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
		await waitFor(() => {
			expect(
				screen.getByText(MockOrganization.display_name),
			).toBeInTheDocument();
			expect(
				screen.getByText(MockOrganization2.display_name),
			).toBeInTheDocument();
		});
	},
};

const collidingDisplayNameOrganizations = [
	{ ...MockOrganization, name: "org-a", display_name: "Dev" },
	{ ...MockOrganization2, name: "org-b", display_name: "Dev" },
	{ ...MockOrganization, id: "org-c", name: "org-c", display_name: "Ops" },
];

export const CollidingDisplayNames: Story = {
	args: {
		value: collidingDisplayNameOrganizations[0],
		options: collidingDisplayNameOrganizations,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: /Dev \(org-a\)/ }),
		).toBeInTheDocument();
		await userEvent.click(canvas.getByRole("button"));
		await waitFor(() => {
			expect(
				screen.getByRole("option", { name: "Dev (org-a)" }),
			).toBeInTheDocument();
			expect(
				screen.getByRole("option", { name: "Dev (org-b)" }),
			).toBeInTheDocument();
			expect(screen.getByRole("option", { name: "Ops" })).toBeInTheDocument();
		});
	},
};

// Collides with an option but is not itself selectable, mirroring a
// deep link to a list-only organization.
const listOnlyOrganization = {
	...MockOrganization,
	id: "org-t",
	name: "org-t",
	display_name: "Dev",
};

export const NonOptionValueDisambiguatesOptions: Story = {
	args: {
		value: listOnlyOrganization,
		options: collidingDisplayNameOrganizations.slice(0, 1),
		labelOrganizations: [
			...collidingDisplayNameOrganizations.slice(0, 1),
			listOnlyOrganization,
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: /Dev \(org-t\)/ }),
		).toBeInTheDocument();
		await userEvent.click(canvas.getByRole("button"));
		await waitFor(() => {
			expect(
				screen.getByRole("option", { name: "Dev (org-a)" }),
			).toBeInTheDocument();
			expect(
				screen.queryByRole("option", { name: /org-t/ }),
			).not.toBeInTheDocument();
		});
	},
};

export const FallbackNameCollision: Story = {
	args: {
		value: null,
		options: [
			{ ...MockOrganization, name: "dev", display_name: "" },
			{ ...MockOrganization2, name: "org-b", display_name: "dev" },
			{ ...MockOrganization, id: "org-c", name: "org-c", display_name: "Ops" },
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		await waitFor(() => {
			expect(screen.getByRole("option", { name: /^dev$/ })).toBeInTheDocument();
			expect(
				screen.getByRole("option", { name: "dev (org-b)" }),
			).toBeInTheDocument();
			expect(screen.getByRole("option", { name: "Ops" })).toBeInTheDocument();
		});
	},
};

export const WithValue: Story = {
	args: {
		value: MockOrganization2,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByText(MockOrganization2.display_name),
			).toBeInTheDocument();
		});
		expect(args.onChange).not.toHaveBeenCalled();
	},
};

export const OneOrg: Story = {
	args: {
		value: MockOrganization,
		options: [MockOrganization],
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByText(MockOrganization.display_name),
			).toBeInTheDocument();
		});
		expect(args.onChange).not.toHaveBeenCalled();
	},
};
