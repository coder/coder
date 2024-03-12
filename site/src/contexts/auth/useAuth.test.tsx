import { renderHook } from "@testing-library/react";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { createTestQueryClient } from "testHelpers/renderHelpers";
import { AuthProvider } from "./AuthProvider";
import { useAuth, useAuthenticated } from "./useAuth";

const Wrapper: FC<PropsWithChildren> = ({ children }) => {
  return (
    <QueryClientProvider client={createTestQueryClient()}>
      <AuthProvider>{children}</AuthProvider>
    </QueryClientProvider>
  );
};

describe("useAuth", () => {
  it("throws an error if it is used outside of <AuthProvider />", () => {
    jest.spyOn(console, "error").mockImplementation(() => {});

    expect(() => {
      renderHook(() => useAuth());
    }).toThrow("useAuth should be used inside of <AuthProvider />");

    jest.restoreAllMocks();
  });

  it("returns AuthContextValue when used inside of <AuthProvider />", () => {
    renderHook(() => useAuth(), {
      wrapper: Wrapper,
    });
  });
});

const RequireAuthWrapper: FC<PropsWithChildren> = ({ children }) => {
  const { user } = useAuth();
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
    jest.restoreAllMocks();
  });
});
