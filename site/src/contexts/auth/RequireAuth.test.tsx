import { MockPermissions, MockUserOwner } from "testHelpers/entities";
import {
	createTestQueryClient,
	renderWithAuth,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { render, renderHook, screen } from "@testing-library/react";
import { useAuthenticated } from "hooks";
import { HttpResponse, http } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { AuthContext, type AuthContextValue } from "./AuthProvider";
import { RequireAuth } from "./RequireAuth";

describe("RequireAuth", () => {
	it("redirects to /login if user is not authenticated", async () => {
		// appear logged out
		server.use(
			http.get("/api/v2/users/me", () => {
				return HttpResponse.json({ message: "no user here" }, { status: 401 });
			}),
		);

		renderWithAuth(<h1>Test</h1>, {
			nonAuthenticatedRoutes: [
				{
					path: "login",
					element: <h1>Login</h1>,
				},
			],
		});

		await screen.findByText("Login");
	});

	it("shows a recoverable error screen on non-401 API errors", () => {
		// Directly mock the auth context with isError=true to simulate
		// a non-401 failure (e.g. 500, network timeout) without relying
		// on real query retry timing.
		const authValue: AuthContextValue = {
			user: undefined,
			isLoading: false,
			isSignedOut: false,
			isSigningOut: false,
			isConfiguringTheFirstUser: false,
			isSignedIn: false,
			isSigningIn: false,
			isUpdatingProfile: false,
			isError: true,
			permissions: undefined,
			signInError: undefined,
			updateProfileError: undefined,
			signOut: vi.fn(),
			signIn: vi.fn(),
			updateProfile: vi.fn(),
		};

		const queryClient = createTestQueryClient();

		render(
			<QueryClientProvider client={queryClient}>
				<AuthContext.Provider value={authValue}>
					<MemoryRouter>
						<Routes>
							<Route element={<RequireAuth />}>
								<Route path="/" element={<h1>Dashboard</h1>} />
							</Route>
						</Routes>
					</MemoryRouter>
				</AuthContext.Provider>
			</QueryClientProvider>,
		);

		// Should show the connection error screen, not the dashboard
		// or the global error boundary.
		expect(screen.getByText("Unable to connect")).toBeDefined();
		expect(screen.getByTestId("retry-button")).toBeDefined();
	});
});

const createAuthWrapper = (override: Partial<AuthContextValue>) => {
	const value = {
		user: undefined,
		isLoading: false,
		isSignedOut: false,
		isSigningOut: false,
		isConfiguringTheFirstUser: false,
		isSignedIn: false,
		isSigningIn: false,
		isUpdatingProfile: false,
		isError: false,
		permissions: undefined,
		signInError: undefined,
		updateProfileError: undefined,
		signOut: vi.fn(),
		signIn: vi.fn(),
		updateProfile: vi.fn(),
		...override,
	};
	const Wrapper: FC<PropsWithChildren> = ({ children }) => {
		return (
			<QueryClientProvider client={createTestQueryClient()}>
				<AuthContext.Provider value={value}>{children}</AuthContext.Provider>
			</QueryClientProvider>
		);
	};

	return Wrapper;
};

describe("useAuthenticated", () => {
	it("throws an error if it is used outside of a context with user", () => {
		vi.spyOn(console, "error").mockImplementation(() => {});

		expect(() => {
			renderHook(() => useAuthenticated(), {
				wrapper: createAuthWrapper({ user: undefined }),
			});
		}).toThrow("User is not authenticated.");

		vi.restoreAllMocks();
	});

	it("throws an error if it is used outside of a context with permissions", () => {
		vi.spyOn(console, "error").mockImplementation(() => {});

		expect(() => {
			renderHook(() => useAuthenticated(), {
				wrapper: createAuthWrapper({ user: MockUserOwner }),
			});
		}).toThrow("Permissions are not available.");

		vi.restoreAllMocks();
	});

	it("returns auth context values for authenticated context", () => {
		expect(() => {
			renderHook(() => useAuthenticated(), {
				wrapper: createAuthWrapper({
					user: MockUserOwner,
					permissions: MockPermissions,
				}),
			});
		}).not.toThrow();
	});
});
