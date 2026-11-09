import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLocation, useNavigate } from "react-router";
import { expect, fn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { AuthContext } from "./AuthProvider";
import { RequireAuth } from "./RequireAuth";

const LoginDestination = () => {
	const location = useLocation();
	return (
		<output aria-label="After login">
			{new URLSearchParams(location.search).get("redirect")}
		</output>
	);
};
const meta = {
	title: "contexts/auth/RequireAuth",
	component: RequireAuth,
	render: function Render() {
		const navigate = useNavigate();
		const location = useLocation();
		return location.hash ? (
			<RequireAuth />
		) : (
			<button
				type="button"
				onClick={() => {
					navigate({
						search: "?tab=ACCESS.md",
						hash: "#remote-desktop-access",
					});
				}}
			>
				Open heading link
			</button>
		);
	},
	decorators: [
		(Story) => (
			<AuthContext.Provider
				value={{
					isLoading: false,
					isSignedOut: true,
					isSigningOut: false,
					isConfiguringTheFirstUser: false,
					isSignedIn: false,
					isSigningIn: false,
					isUpdatingProfile: false,
					user: undefined,
					permissions: undefined,
					signInError: undefined,
					updateProfileError: undefined,
					signOut: fn(),
					signIn: fn(),
					updateProfile: fn(),
				}}
			>
				<Story />
			</AuthContext.Provider>
		),
	],
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/templates/windows/docs",
			},
			routing: [
				{ path: "/templates/windows/docs", useStoryElement: true },
				{ path: "/login", element: <LoginDestination /> },
			],
		}),
	},
} satisfies Meta<typeof RequireAuth>;
export default meta;
type Story = StoryObj<typeof meta>;
export const HeadingLink: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Open heading link" }),
		);
		await expect(
			await within(canvasElement).findByRole("status", { name: "After login" }),
		).toHaveTextContent(
			"/templates/windows/docs?tab=ACCESS.md#remote-desktop-access",
		);
	},
};
