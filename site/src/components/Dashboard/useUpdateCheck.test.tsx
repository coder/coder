import { act, renderHook, waitFor } from "@testing-library/react";
import { useUpdateCheck } from "./useUpdateCheck";
import { QueryClient, QueryClientProvider } from "react-query";
import { ReactNode } from "react";
import { rest } from "msw";
import { MockUpdateCheck } from "testHelpers/entities";
import { server } from "testHelpers/server";

const createWrapper = () => {
  const queryClient = new QueryClient();
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
};

beforeEach(() => {
  window.localStorage.clear();
});

it("is dismissed when does not have permission to see it", () => {
  const { result } = renderHook(() => useUpdateCheck(false), {
    wrapper: createWrapper(),
  });
  expect(result.current.isVisible).toBeFalsy();
});

it("is dismissed when it is already using current version", async () => {
  server.use(
    rest.get("/api/v2/updatecheck", (req, res, ctx) => {
      return res(
        ctx.status(200),
        ctx.json({
          ...MockUpdateCheck,
          current: true,
        }),
      );
    }),
  );
  const { result } = renderHook(() => useUpdateCheck(true), {
    wrapper: createWrapper(),
  });

  await waitFor(() => {
    expect(result.current.isVisible).toBeFalsy();
  });
});

it("is dismissed when it was dismissed previously", async () => {
  server.use(
    rest.get("/api/v2/updatecheck", (req, res, ctx) => {
      return res(
        ctx.status(200),
        ctx.json({
          ...MockUpdateCheck,
          current: false,
        }),
      );
    }),
  );
  window.localStorage.setItem("dismissedVersion", MockUpdateCheck.version);
  const { result } = renderHook(() => useUpdateCheck(true), {
    wrapper: createWrapper(),
  });

  await waitFor(() => {
    expect(result.current.isVisible).toBeFalsy();
  });
});

it("shows when has permission and is outdated", async () => {
  server.use(
    rest.get("/api/v2/updatecheck", (req, res, ctx) => {
      return res(
        ctx.status(200),
        ctx.json({
          ...MockUpdateCheck,
          current: false,
        }),
      );
    }),
  );
  const { result } = renderHook(() => useUpdateCheck(true), {
    wrapper: createWrapper(),
  });

  await waitFor(() => {
    expect(result.current.isVisible).toBeTruthy();
  });
});

it("shows when has permission and is outdated", async () => {
  server.use(
    rest.get("/api/v2/updatecheck", (req, res, ctx) => {
      return res(
        ctx.status(200),
        ctx.json({
          ...MockUpdateCheck,
          current: false,
        }),
      );
    }),
  );
  const { result } = renderHook(() => useUpdateCheck(true), {
    wrapper: createWrapper(),
  });

  await waitFor(() => {
    expect(result.current.isVisible).toBeTruthy();
  });

  act(() => {
    result.current.dismiss();
  });

  await waitFor(() => {
    expect(result.current.isVisible).toBeFalsy();
  });
  expect(localStorage.getItem("dismissedVersion")).toEqual(
    MockUpdateCheck.version,
  );
});
