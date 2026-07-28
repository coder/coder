import { type FC, type ReactNode, Suspense } from "react";
import { Outlet } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { cn } from "#/utils/cn";

interface SidebarLayoutProps {
	sidebar: ReactNode;
}

/**
 * Full-bleed settings shell with a left sidebar rail and bordered
 * divider. Used by deployment, AI, organization, and logs settings.
 *
 * The outer rail is at least viewport-tall (minus navbar + banner) so
 * the border always reaches the stats bar. The inner nav sticks and
 * scrolls when the links overflow.
 */
export const SidebarLayout: FC<SidebarLayoutProps> = ({ sidebar }) => {
	return (
		<div className="flex flex-col lg:flex-row flex-1 min-h-0">
			<div
				className={cn(
					"border-0 border-r border-solid border-border flex-shrink-0",
					// Navbar 72px + deployment stats banner h-9 (2.25rem).
					"lg:min-h-[calc(100vh-72px-2.25rem)]",
				)}
			>
				<div
					className={cn(
						"px-3 pt-6 pb-6",
						"lg:sticky lg:top-[72px] lg:max-h-[calc(100vh-72px-2.25rem)] lg:overflow-y-auto lg:[scrollbar-gutter:stable]",
					)}
				>
					{sidebar}
				</div>
			</div>
			<div className="grow min-w-0 pt-6 pb-10 px-10">
				<Suspense fallback={<Loader />}>
					<Outlet />
				</Suspense>
			</div>
		</div>
	);
};
