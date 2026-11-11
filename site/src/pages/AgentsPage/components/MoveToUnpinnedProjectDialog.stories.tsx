import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect } from "react";
import { MockChatProject } from "#/testHelpers/entities";
import {
	MoveToUnpinnedProjectDialogProvider,
	useShowMoveToUnpinnedProjectDialog,
} from "./MoveToUnpinnedProjectDialog";

// Stands in for a move that just finished into an unpinned project.
const MovedChat: React.FC = () => {
	const show = useShowMoveToUnpinnedProjectDialog();
	useEffect(() => show?.(MockChatProject), [show]);
	return null;
};

const meta: Meta<typeof MoveToUnpinnedProjectDialogProvider> = {
	title: "pages/AgentsPage/MoveToUnpinnedProjectDialog",
	component: MoveToUnpinnedProjectDialogProvider,
	args: { children: <MovedChat /> },
};

export default meta;
type Story = StoryObj<typeof MoveToUnpinnedProjectDialogProvider>;

export const Default: Story = {};
