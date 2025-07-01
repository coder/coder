import "./theme/globalFonts";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import {
	type FC,
	type ReactNode,
	StrictMode,
	useEffect,
	useState,
} from "react";
import { HelmetProvider } from "react-helmet-async";
import { QueryClient, QueryClientProvider } from "react-query";
import { RouterProvider } from "react-router-dom";
import { GlobalSnackbar } from "./components/GlobalSnackbar/GlobalSnackbar";
import { ThemeProvider } from "./contexts/ThemeProvider";
import { AuthProvider } from "./contexts/auth/AuthProvider";
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
		// Storing key in variable to avoid accidental typos; we're working with the
		// window object, so there's basically zero type-checking available
		const toggleKey = "toggleDevtools";

		// Don't want to throw away the previous devtools value if some other
		// extension added something already
		const devtoolsBeforeSync = window[toggleKey];
		window[toggleKey] = () => {
			devtoolsBeforeSync?.();
			setShowDevtools((current) => !current);
		};

		return () => {
			window[toggleKey] = devtoolsBeforeSync;
		};
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
		<StrictMode>
			<AppProviders>
				{/* If you're wondering where the global error boundary is,
				    it's connected to the router */}
				<RouterProvider router={router} />
			</AppProviders>
		</StrictMode>
	);
};
