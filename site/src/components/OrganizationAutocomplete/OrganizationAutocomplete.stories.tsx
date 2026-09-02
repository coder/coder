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

export const ActiveSortsFirstThenAlphabetical: Story = {
	args: {
		value: MockOrganization2,
		options: [MockOrganization, MockOrganization2],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		const options = await screen.findAllByRole("option");
		expect(options[0]).toHaveTextContent(MockOrganization2.display_name);
		expect(options[1]).toHaveTextContent(MockOrganization.display_name);
	},
};

export const TabbableOptions: Story = {
	args: {
		value: MockOrganization,
		optionsTabbable: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		const options = await screen.findAllByRole("option");
		await userEvent.tab();
		expect(options[0]).toHaveFocus();
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
