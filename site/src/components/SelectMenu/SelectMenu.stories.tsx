import { withDesktopViewport } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Avatar } from "components/Avatar/Avatar";
import { action } from "storybook/actions";
import { userEvent, within } from "storybook/test";
import {
	SelectMenu,
	SelectMenuButton,
	SelectMenuContent,
	SelectMenuIcon,
	SelectMenuItem,
	SelectMenuList,
	SelectMenuSearch,
	SelectMenuTrigger,
} from "./SelectMenu";

const meta: Meta<typeof SelectMenu> = {
	title: "components/SelectMenu",
	component: SelectMenu,
	render: function SelectMenuRender() {
		const opts = options(50);
		const selectedOpt = opts[20];

		return (
			<SelectMenu>
				<SelectMenuTrigger>
					<SelectMenuButton
						startIcon={<Avatar size="sm" fallback={selectedOpt} />}
					>
						{selectedOpt}
					</SelectMenuButton>
				</SelectMenuTrigger>
				<SelectMenuContent>
					<SelectMenuSearch onChange={() => {}} />
					<SelectMenuList>
						{opts.map((o) => (
							<SelectMenuItem key={o} selected={o === selectedOpt}>
								<SelectMenuIcon>
									<Avatar size="sm" fallback={o} />
								</SelectMenuIcon>
								{o}
							</SelectMenuItem>
						))}
					</SelectMenuList>
				</SelectMenuContent>
			</SelectMenu>
		);
	},
	decorators: [withDesktopViewport],
};

function options(n: number): string[] {
	return Array.from({ length: n }, (_, i) => `Item ${i + 1}`);
}

export default meta;
type Story = StoryObj<typeof SelectMenu>;

export const Closed: Story = {};

export const Open: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
	},
};

export const LongButtonText: Story = {
	render: function SelectMenuRender() {
		const longOption = "Very long text that should be truncated";
		const opts = [...options(50), longOption];
		const selectedOpt = longOption;

		return (
			<SelectMenu>
				<SelectMenuTrigger>
					<SelectMenuButton
						className="w-48"
						startIcon={<Avatar size="sm" fallback={selectedOpt} />}
					>
						{selectedOpt}
					</SelectMenuButton>
				</SelectMenuTrigger>
				<SelectMenuContent>
					<SelectMenuSearch onChange={() => {}} />
					<SelectMenuList>
						{opts.map((o) => (
							<SelectMenuItem key={o} selected={o === selectedOpt}>
								<SelectMenuIcon>
									<Avatar size="sm" fallback={o} />
								</SelectMenuIcon>
								{o}
							</SelectMenuItem>
						))}
					</SelectMenuList>
				</SelectMenuContent>
			</SelectMenu>
		);
	},
};

export const NoSelectedOption: Story = {
	render: function SelectMenuRender() {
		const opts = options(50);

		return (
			<SelectMenu>
				<SelectMenuTrigger>
					<SelectMenuButton className="w-48">All users</SelectMenuButton>
				</SelectMenuTrigger>
				<SelectMenuContent>
					<SelectMenuSearch onChange={action("search")} />
					<SelectMenuList>
						{opts.map((o) => (
							<SelectMenuItem key={o}>
								<SelectMenuIcon>
									<Avatar size="sm" fallback={o} />
								</SelectMenuIcon>
								{o}
							</SelectMenuItem>
						))}
					</SelectMenuList>
				</SelectMenuContent>
			</SelectMenu>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
	},
};
