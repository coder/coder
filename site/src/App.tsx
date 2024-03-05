import "./theme/globalFonts";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import { type FC, type ReactNode, useEffect, useState } from "react";
import { HelmetProvider } from "react-helmet-async";
import { QueryClient, QueryClientProvider } from "react-query";
import { RouterProvider } from "react-router-dom";
import { ErrorBoundary } from "./components/ErrorBoundary/ErrorBoundary";
import { GlobalSnackbar } from "./components/GlobalSnackbar/GlobalSnackbar";
import { AuthProvider } from "./contexts/auth/AuthProvider";
import { ThemeProvider } from "./contexts/ThemeProvider";
import { router } from "./router";

const defaultQueryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      refetchOnWindowFocus: false,
    },
  },
});

interface AppProvidersProps {
  children: ReactNode;
  queryClient?: QueryClient;
}

// extending the global window interface so we can conditionally
// show our react query devtools
declare global {
  interface Window {
    toggleDevtools: () => void;
  }
}

export const AppProviders: FC<AppProvidersProps> = ({
  children,
  queryClient = defaultQueryClient,
}) => {
  // https://tanstack.com/query/v4/docs/react/devtools
  const [showDevtools, setShowDevtools] = useState(false);
  useEffect(() => {
    window.toggleDevtools = () => setShowDevtools((old) => !old);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- no dependencies needed here
  }, []);

  return (
    <HelmetProvider>
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
          <ThemeProvider>
            {children}
            <GlobalSnackbar />
          </ThemeProvider>
        </AuthProvider>
        {showDevtools && <ReactQueryDevtools initialIsOpen={showDevtools} />}
      </QueryClientProvider>
    </HelmetProvider>
  );
};

export const App: FC = () => {
  return (
    <AppProviders>
      <ErrorBoundary>
        <RouterProvider router={router} />
      </ErrorBoundary>
    </AppProviders>
  );
};
