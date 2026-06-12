import { ChevronRightIcon } from "lucide-react";
import { type FC, useState } from "react";
import { NavLink } from "react-router";
import { cn } from "#/utils/cn";
import { type DocsManifestRoute, manifestPathToRoute } from "./docsContent";

interface DocsSidebarProps {
	routes: readonly DocsManifestRoute[];
	activeRoute: string;
}

export const DocsSidebar: FC<DocsSidebarProps> = ({ routes, activeRoute }) => {
	return (
		<nav
			aria-label="Documentation"
			className="w-64 shrink-0 overflow-y-auto border-0 border-r border-solid border-border py-6 pr-4"
		>
			<DocsNavTree routes={routes} activeRoute={activeRoute} depth={0} />
		</nav>
	);
};

interface DocsNavTreeProps {
	routes: readonly DocsManifestRoute[];
	activeRoute: string;
	depth: number;
}

const DocsNavTree: FC<DocsNavTreeProps> = ({ routes, activeRoute, depth }) => {
	return (
		<ul className="m-0 list-none p-0">
			{routes.map((route) => (
				<DocsNavItem
					key={route.path}
					route={route}
					activeRoute={activeRoute}
					depth={depth}
				/>
			))}
		</ul>
	);
};

interface DocsNavItemProps {
	route: DocsManifestRoute;
	activeRoute: string;
	depth: number;
}

const DocsNavItem: FC<DocsNavItemProps> = ({ route, activeRoute, depth }) => {
	const routePath = manifestPathToRoute(route.path);
	const isAncestorOfActive =
		routePath !== "" &&
		(activeRoute === routePath || activeRoute.startsWith(`${routePath}/`));
	// The user's manual toggle wins once touched; otherwise expansion
	// follows the active route so deep links and in-content navigation
	// reveal their section.
	const [manualExpanded, setManualExpanded] = useState<boolean | null>(null);
	const expanded = manualExpanded ?? isAncestorOfActive;
	const hasChildren = (route.children?.length ?? 0) > 0;

	return (
		<li>
			<div className="flex items-center">
				<NavLink
					end
					to={routePath === "" ? "/docs" : `/docs/${routePath}`}
					className={({ isActive }) =>
						cn(
							"block grow rounded-md px-2 py-1.5 text-sm no-underline",
							"text-content-secondary hover:text-content-primary",
							isActive && "bg-surface-secondary text-content-primary",
						)
					}
					style={{ paddingLeft: 8 + depth * 12 }}
				>
					{route.title}
				</NavLink>
				{hasChildren && (
					<button
						type="button"
						aria-label={`Toggle ${route.title} section`}
						aria-expanded={expanded}
						onClick={() => setManualExpanded(!expanded)}
						className="flex cursor-pointer items-center border-none bg-transparent p-1 text-content-secondary"
					>
						<ChevronRightIcon
							className={cn(
								"size-4 transition-transform",
								expanded && "rotate-90",
							)}
						/>
					</button>
				)}
			</div>
			{hasChildren && expanded && route.children && (
				<DocsNavTree
					routes={route.children}
					activeRoute={activeRoute}
					depth={depth + 1}
				/>
			)}
		</li>
	);
};
