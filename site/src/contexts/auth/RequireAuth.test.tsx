import { renderHook, screen } from "@testing-library/react";
import { rest } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MockPermissions, MockUser } from "testHelpers/entities";
import {
  createTestQueryClient,
  renderWithAuth,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { AuthContext, type AuthContextValue } from "./AuthProvider";
import { useAuthenticated } from "./RequireAuth";

describe("RequireAuth", () => {
  it("redirects to /login if user is not authenticated", async () => {
    // appear logged out
    server.use(
      rest.get("/api/v2/users/me", (req, res, ctx) => {
        return res(ctx.status(401), ctx.json({ message: "no user here" }));
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
    permissions: undefined,
    authMethods: undefined,
    organizationId: undefined,
    signInError: undefined,
    updateProfileError: undefined,
    signOut: jest.fn(),
    signIn: jest.fn(),
    updateProfile: jest.fn(),
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
    jest.spyOn(console, "error").mockImplementation(() => {});

    expect(() => {
      renderHook(() => useAuthenticated(), {
        wrapper: createAuthWrapper({ user: undefined }),
      });
    }).toThrow("User is not authenticated.");

    jest.restoreAllMocks();
  });

  it("throws an error if it is used outside of a context with permissions", () => {
    jest.spyOn(console, "error").mockImplementation(() => {});

    expect(() => {
      renderHook(() => useAuthenticated(), {
        wrapper: createAuthWrapper({ user: MockUser }),
      });
    }).toThrow("Permissions are not available.");

    jest.restoreAllMocks();
  });

  it("returns auth context values for authenticated context", () => {
    expect(() => {
      renderHook(() => useAuthenticated(), {
        wrapper: createAuthWrapper({
          user: MockUser,
          permissions: MockPermissions,
        }),
      });
    }).not.toThrow();
  });
});
