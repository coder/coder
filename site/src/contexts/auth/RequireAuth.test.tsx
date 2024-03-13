import { renderHook, screen } from "@testing-library/react";
import { rest } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import {
  createTestQueryClient,
  renderWithAuth,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { AuthProvider, useAuthContext } from "./AuthProvider";
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

const Wrapper: FC<PropsWithChildren> = ({ children }) => {
  return (
    <QueryClientProvider client={createTestQueryClient()}>
      <AuthProvider>{children}</AuthProvider>
    </QueryClientProvider>
  );
};

const RequireAuthWrapper: FC<PropsWithChildren> = ({ children }) => {
  const { user } = useAuthContext();
  return user ? <>{children}</> : null;
};

describe("useAuthenticated", () => {
  it("throws an error if it is used outside of an authenticated context", () => {
    jest.spyOn(console, "error").mockImplementation(() => {});

    expect(() => {
      renderHook(() => useAuthenticated(), { wrapper: Wrapper });
    }).toThrow("User is not authenticated.");

    jest.restoreAllMocks();
  });

  it("returns auth context values for authenticated context", () => {
    renderHook(() => useAuthenticated(), {
      wrapper: ({ children }) => (
        <Wrapper>
          <RequireAuthWrapper>{children}</RequireAuthWrapper>
        </Wrapper>
      ),
    });
  });
});
