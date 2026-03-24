import { API } from "api/api";
import { isApiError } from "api/errors";
import { Button } from "components/Button/Button";
import { CoderIcon } from "components/Icons/CoderIcon";
import { Loader } from "components/Loader/Loader";
import { ProxyProvider as ProductionProxyProvider } from "contexts/ProxyContext";
import { DashboardProvider as ProductionDashboardProvider } from "modules/dashboard/DashboardProvider";
import { type FC, useEffect } from "react";
import { Navigate, Outlet, useLocation } from "react-router";
import { embedRedirect } from "utils/redirect";
import { useAuthContext } from "./AuthProvider";

type RequireAuthProps = Readonly<{
	ProxyProvider?: typeof ProductionProxyProvider;
	DashboardProvider?: typeof ProductionDashboardProvider;
}>;

/**
 * Wraps any component and ensures that the user has been authenticated before
 * they can access the component's contents.
 *
 * In production, it is assumed that this component will not be called with any
 * props at all. But to make testing easier, you can call this component with
 * specific providers to mock them out.
 */
export const RequireAuth: FC<RequireAuthProps> = ({
	DashboardProvider = ProductionDashboardProvider,
	ProxyProvider = ProductionProxyProvider,
}) => {
	const location = useLocation();
	const { signOut, isSigningOut, isSignedOut, isSignedIn, isLoading, isError } =
		useAuthContext();

	useEffect(() => {
		if (isLoading || isSigningOut || !isSignedIn) {
			return;
		}

		const axiosInstance = API.getAxiosInstance();
		const interceptorHandle = axiosInstance.interceptors.response.use(
			(okResponse) => okResponse,
			(error: unknown) => {
				// 401 Unauthorized
				// If we encountered an authentication error, then our token is probably
				// invalid and we should update the auth state to reflect that.
				if (isApiError(error) && error.response.status === 401) {
					signOut();
				}

				// Otherwise, pass the response through so that it can be displayed in
				// the UI
				return Promise.reject(error);
			},
		);

		return () => {
			axiosInstance.interceptors.response.eject(interceptorHandle);
		};
	}, [isLoading, isSigningOut, isSignedIn, signOut]);

	if (isLoading || isSigningOut) {
		return <Loader fullscreen />;
	}

	if (isSignedOut) {
		const isHomePage = location.pathname === "/";
		const navigateTo = isHomePage
			? "/login"
			: embedRedirect(`${location.pathname}${location.search}`);

		return (
			<Navigate to={navigateTo} state={{ isRedirect: !isHomePage }} replace />
		);
	}

	if (isError) {
		return <ConnectionErrorScreen />;
	}

	// Authenticated pages have access to some contexts for knowing enabled experiments
	// and where to route workspace connections.
	return (
		<DashboardProvider>
			<ProxyProvider>
				<Outlet />
			</ProxyProvider>
		</DashboardProvider>
	);
};

/**
 * Full-screen error shown when the user API call fails with a non-401
 * error (network timeout, 500, 502, etc.). Gives the user a simple
 * retry action instead of crashing to the global error boundary.
 */
const ConnectionErrorScreen: FC = () => {
	return (
		<div className="bg-surface-primary text-center w-full h-full flex justify-center items-center absolute inset-0">
			<main className="flex gap-6 w-full max-w-prose p-4 flex-col flex-nowrap">
				<div className="flex gap-2 flex-col items-center">
					<CoderIcon className="w-11 h-11" />
					<div className="text-content-primary flex flex-col gap-1">
						<h1 className="text-2xl font-semibold m-0">Unable to connect</h1>
						<p className="leading-6 m-0 text-content-secondary text-sm">
							We&apos;re having trouble reaching the server. This may be a
							temporary network issue.
						</p>
					</div>
				</div>

				<div className="flex flex-row flex-nowrap justify-center gap-2">
					<Button
						className="min-w-32"
						onClick={() => window.location.reload()}
						data-testid="retry-button"
					>
						Retry
					</Button>
				</div>
			</main>
		</div>
	);
};
