import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { templateVersionsQueryKey } from "#/api/queries/templates";
import {
	MockTemplateVersion,
	MockTemplateVersionWithMarkdownMessage,
	MockWorkspace,
} from "#/testHelpers/entities";
import { ChangeWorkspaceVersionDialog } from "./ChangeWorkspaceVersionDialog";

const noMessage = {
	...MockTemplateVersion,
	name: "no-message",
	id: "no-message",
	message: "",
};

const meta: Meta<typeof ChangeWorkspaceVersionDialog> = {
	title: "modules/workspaces/ChangeWorkspaceVersionDialog",
	component: ChangeWorkspaceVersionDialog,
	args: {
		open: true,
		workspace: MockWorkspace,
	},
	parameters: {
		queries: [
			{
				key: templateVersionsQueryKey(MockWorkspace.template_id),
				data: [
					MockTemplateVersion,
					MockTemplateVersionWithMarkdownMessage,
					noMessage,
				],
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof ChangeWorkspaceVersionDialog>;

export const CurrentVersion: Story = {};

export const NoMessage: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				template_version_id: noMessage.id,
			},
		},
	},
};

export const TextMessage: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				template_version_id: MockTemplateVersion.id,
			},
		},
	},
};

export const MarkdownMessage: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				template_version_id: MockTemplateVersionWithMarkdownMessage.id,
			},
		},
	},
};

export const SelectVersion: Story = {
	args: {
		onClose: fn(),
		onConfirm: fn(),
	},
	play: async ({ args }) => {
		const body = within(document.body);

		const trigger = body.getByRole("button", { name: "Template version" });
		await userEvent.click(trigger);

		const option = await body.findByRole("option", {
			name: new RegExp(MockTemplateVersionWithMarkdownMessage.name),
		});
		await waitFor(() => expect(isTopmostAtCenter(option)).toBe(true));

		await userEvent.click(option);
		await waitFor(() =>
			expect(trigger).toHaveTextContent(
				MockTemplateVersionWithMarkdownMessage.name,
			),
		);

		await userEvent.click(body.getByRole("button", { name: "Change" }));
		await waitFor(() =>
			expect(args.onConfirm).toHaveBeenCalledWith(
				MockTemplateVersionWithMarkdownMessage,
			),
		);
	},
};

/**
 * Reports whether a click at the element's center would land on it.
 *
 * The version picker is a popover that portals to the document body, outside
 * the dialog it belongs to. Queries and visibility assertions still pass when
 * that popover paints underneath the dialog surface, so only a hit test proves
 * the options are reachable.
 */
function isTopmostAtCenter(element: Element): boolean {
	const { left, top, width, height } = element.getBoundingClientRect();
	const hit = document.elementFromPoint(
		Math.round(left + width / 2),
		Math.round(top + height / 2),
	);
	return hit !== null && element.contains(hit);
}
