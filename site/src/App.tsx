import { QueryClient, QueryClientProvider } from "react-query";
import type { FC, ReactNode } from "react";
import { HelmetProvider } from "react-helmet-async";
import { AppRouter } from "./AppRouter";
import { ThemeProviders } from "./contexts/ThemeProviders";
import { AuthProvider } from "./contexts/AuthProvider/AuthProvider";
import { ErrorBoundary } from "./components/ErrorBoundary/ErrorBoundary";
import { GlobalSnackbar } from "./components/GlobalSnackbar/GlobalSnackbar";
import "./theme/globalFonts";

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

export const AppProviders: FC<AppProvidersProps> = ({
  children,
  queryClient = defaultQueryClient,
}) => {
  return (
    <HelmetProvider>
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
          <ThemeProviders>
            {children}
            <GlobalSnackbar />
          </ThemeProviders>
        </AuthProvider>
      </QueryClientProvider>
    </HelmetProvider>
  );
};

export const App: FC = () => {
  return (
    <AppProviders>
      <ErrorBoundary>
        <AppRouter />
      </ErrorBoundary>
    </AppProviders>
  );
};
