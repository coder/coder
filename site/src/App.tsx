import "./theme/globalFonts";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import {
	type FC,
	type ReactNode,
	StrictMode,
	useEffect,
	useState,
} from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { RouterProvider } from "react-router";
import { GlobalSnackbar } from "./components/GlobalSnackbar/GlobalSnackbar";
import { TooltipProvider } from "./components/Tooltip/Tooltip";
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

export const AppProviders: FC<AppProvidersProps> = ({
	children,
	queryClient = defaultQueryClient,
}) => {
	// https://tanstack.com/query/v4/docs/react/devtools
	const [showDevtools, setShowDevtools] = useState(false);

	useEffect(() => {
		// Don't want to throw away the previous devtools value if some other
		// extension added something already
		const devtoolsBeforeSync = window.toggleDevtools;
		window.toggleDevtools = () => {
			devtoolsBeforeSync?.();
			setShowDevtools((current) => !current);
		};

		return () => {
			window.toggleDevtools = devtoolsBeforeSync;
		};
	}, []);

	return (
		<QueryClientProvider client={queryClient}>
			<AuthProvider>
				<ThemeProvider>
					<TooltipProvider>
						{children}
						<GlobalSnackbar />
					</TooltipProvider>
				</ThemeProvider>
			</AuthProvider>
			{showDevtools && <ReactQueryDevtools initialIsOpen={showDevtools} />}
		</QueryClientProvider>
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
