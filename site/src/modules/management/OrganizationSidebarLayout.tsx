import { Loader } from "components/Loader/Loader";
import { type FC, Suspense, useEffect, useRef } from "react";
import { Outlet, useLocation } from "react-router";
import { OrganizationSidebar } from "./OrganizationSidebar";

const OrganizationSidebarLayout: FC = () => {
	const location = useLocation();
	const mainRef = useRef<HTMLElement>(null);

	// Reset scroll position when navigating between sub-pages.
	// biome-ignore lint/correctness/useExhaustiveDependencies: scroll on pathname change
	useEffect(() => {
		mainRef.current?.scrollTo(0, 0);
	}, [location.pathname]);

	return (
		<div className="flex flex-row flex-1 min-h-0 w-full">
			<OrganizationSidebar />
			<main
				ref={mainRef}
				className="flex flex-col items-center flex-1 min-h-0 h-full overflow-y-auto w-full px-10 pt-10"
			>
				<Suspense fallback={<Loader />}>
					<Outlet />
				</Suspense>
			</main>
		</div>
	);
};

export default OrganizationSidebarLayout;
