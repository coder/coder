import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { userSecrets } from "#/api/queries/userSecrets";
import type { UserSecret } from "#/api/typesGenerated";
import { MockUserOwner, MockUserSecrets } from "#/testHelpers/entities";
import { withAuthProvider, withWebSocket } from "#/testHelpers/storybook";
import SecretsPage from "./SecretsPage";

const secretWithName = (secret: UserSecret, name: string): UserSecret => ({
	...secret,
	id: name,
	name,
	env_name: `${name.toUpperCase().replaceAll("-", "_")}_ENV`,
	file_path: "",
});

const initialSecret = secretWithName(MockUserSecrets[0], "initial-secret");
const openedSocketSecret = secretWithName(
	MockUserSecrets[1],
	"opened-socket-secret",
);
const refreshedSecret = secretWithName(MockUserSecrets[1], "refreshed-secret");
const watchUserSecretsRoute = `/api/v2/users/${MockUserOwner.id}/secrets/-/watch`;

const setupSecretRefresh = (secrets: readonly UserSecret[]) => {
	spyOn(API, "getUserSecrets").mockResolvedValue([...secrets]);
};

const meta = {
	title: "pages/UserSettingsPage/SecretsPage",
	component: SecretsPage,
	decorators: [withAuthProvider, withWebSocket],
	parameters: {
		user: MockUserOwner,
		queries: [
			{
				key: userSecrets(MockUserOwner.id).queryKey,
				data: [initialSecret],
			},
		],
	},
} satisfies Meta<typeof SecretsPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const RefreshesOnSocketOpen: Story = {
	beforeEach: () => {
		setupSecretRefresh([openedSocketSecret]);
	},
	parameters: {
		webSocket: {
			[watchUserSecretsRoute]: [{ event: "open" }],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			await canvas.findByText(openedSocketSecret.name),
		).toBeVisible();
		await waitFor(() => {
			expect(API.getUserSecrets).toHaveBeenCalledWith(MockUserOwner.id);
		});
	},
};

export const RefreshesOnMatchingEvent: Story = {
	beforeEach: () => {
		setupSecretRefresh([refreshedSecret]);
	},
	parameters: {
		webSocket: {
			[watchUserSecretsRoute]: [
				{
					event: "message",
					data: JSON.stringify({
						kind: "updated",
						user_id: MockUserOwner.id,
						name: initialSecret.name,
					}),
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(await canvas.findByText(refreshedSecret.name)).toBeVisible();
		await waitFor(() => {
			expect(API.getUserSecrets).toHaveBeenCalledWith(MockUserOwner.id);
		});
	},
};
