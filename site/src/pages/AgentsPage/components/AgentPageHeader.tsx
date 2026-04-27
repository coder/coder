import {
	ArrowLeftIcon,
	BarChart3Icon,
	BellIcon,
	BellOffIcon,
	EllipsisIcon,
	PanelLeftIcon,
	SettingsIcon,
	Volume2Icon,
	VolumeOffIcon,
} from "lucide-react";
import type { FC, ReactNode } from "react";
import { useEffect, useState } from "react";
import { Link, NavLink, useLocation, useOutletContext } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { CoderIcon } from "#/components/Icons/CoderIcon";
import { Spinner } from "#/components/Spinner/Spinner";
import { useWebpushNotifications } from "#/contexts/useWebpushNotifications";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import type { AgentsOutletContext } from "../AgentsPageView";
import { getChimeEnabled, setChimeEnabled } from "../utils/chime";

interface AgentPageHeaderProps {
	children?: ReactNode;
	/** When set, shows a back link on mobile instead of the logo
	 *  and hides the settings/analytics nav buttons. */
	mobileBack?: { to: string; label: string };
	chimeEnabled?: boolean;
	onToggleChime?: () => void;
	webPush?: ReturnType<typeof useWebpushNotifications>;
	onToggleNotifications?: () => Promise<void> | void;
}

export const AgentPageHeader: FC<AgentPageHeaderProps> = ({
	children,
	mobileBack,
	chimeEnabled: controlledChimeEnabled,
	onToggleChime,
	webPush: controlledWebPush,
	onToggleNotifications,
}) => {
	const { isSidebarCollapsed, onExpandSidebar } =
		useOutletContext<AgentsOutletContext>();
	const { appearance } = useDashboard();
	const logoUrl = appearance.logo_url;
	const location = useLocation();

	const [internalChimeEnabled, setInternalChimeEnabled] =
		useState(getChimeEnabled);
	const internalWebPush = useWebpushNotifications();
	const chimeEnabled = controlledChimeEnabled ?? internalChimeEnabled;
	const webPush = controlledWebPush ?? internalWebPush;
	const [isDesktop, setIsDesktop] = useState<boolean>(() => {
		return window.matchMedia("(min-width: 768px)").matches;
	});

	useEffect(() => {
		const mediaQuery = window.matchMedia("(min-width: 768px)");
		const onMediaChange = (event: MediaQueryListEvent) => {
			setIsDesktop(event.matches);
		};

		setIsDesktop(mediaQuery.matches);
		if (typeof mediaQuery.addEventListener === "function") {
			mediaQuery.addEventListener("change", onMediaChange);
		} else {
			mediaQuery.addListener(onMediaChange);
		}
		return () => {
			if (typeof mediaQuery.removeEventListener === "function") {
				mediaQuery.removeEventListener("change", onMediaChange);
			} else {
				mediaQuery.removeListener(onMediaChange);
			}
		};
	}, []);

	const handleChimeToggle = () => {
		if (onToggleChime) {
			onToggleChime();
			return;
		}
		const next = !chimeEnabled;
		setInternalChimeEnabled(next);
		setChimeEnabled(next);
	};

	const handleNotificationToggle = async () => {
		if (onToggleNotifications) {
			await onToggleNotifications();
			return;
		}
		try {
			if (webPush.subscribed) {
				await webPush.unsubscribe();
			} else {
				await webPush.subscribe();
			}
		} catch (error) {
			const action = webPush.subscribed ? "disable" : "enable";
			toast.error(getErrorMessage(error, `Failed to ${action} notifications.`));
		}
	};

	return (
		<div className="order-first flex shrink-0 items-center gap-2 pl-4 pr-2 pt-3 pb-0.5 md:order-none md:px-4 md:py-0.5">
			{mobileBack ? (
				<Button
					asChild
					variant="subtle"
					size="icon"
					aria-label={mobileBack.label}
					className="h-7 w-7 shrink-0 md:hidden"
				>
					<Link to={mobileBack.to}>
						<ArrowLeftIcon />
					</Link>
				</Button>
			) : (
				<NavLink to="/workspaces" className="inline-flex shrink-0 md:hidden">
					{logoUrl ? (
						<ExternalImage className="h-6" src={logoUrl} alt="Logo" />
					) : (
						<CoderIcon className="h-6 w-6 fill-content-primary" />
					)}
				</NavLink>
			)}
			{isSidebarCollapsed && (
				<Button
					variant="subtle"
					size="icon"
					onClick={onExpandSidebar}
					aria-label="Expand sidebar"
					className="hidden h-7 w-7 min-w-0 shrink-0 md:inline-flex"
				>
					<PanelLeftIcon />
				</Button>
			)}
			<div className="min-w-0 flex-1" />
			{children && isDesktop && (
				<div className="hidden items-center gap-2 md:flex">{children}</div>
			)}
			{/* Mobile: meatball menu with all actions */}
			{!mobileBack && !isDesktop && (
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							variant="subtle"
							size="icon"
							aria-label="More options"
							className="h-7 w-7 text-content-secondary hover:text-content-primary md:hidden"
						>
							<EllipsisIcon />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent
						align="end"
						className="mobile-full-width-dropdown mobile-full-width-dropdown-top [&_[role=menuitem]]:text-sm"
					>
						<DropdownMenuItem asChild>
							<Link to="/agents/settings" state={{ from: location.pathname }}>
								<SettingsIcon className="size-icon-sm" />
								Settings
							</Link>
						</DropdownMenuItem>
						<DropdownMenuItem asChild>
							<Link to="/agents/analytics">
								<BarChart3Icon className="size-icon-sm" />
								Analytics
							</Link>
						</DropdownMenuItem>
						<DropdownMenuItem
							onSelect={(e) => {
								e.preventDefault();
								handleChimeToggle();
							}}
						>
							{chimeEnabled ? (
								<Volume2Icon className="size-icon-sm" />
							) : (
								<VolumeOffIcon className="size-icon-sm" />
							)}
							{chimeEnabled ? "Turn sound off" : "Turn sound on"}
						</DropdownMenuItem>
						{webPush.enabled && (
							<DropdownMenuItem
								onSelect={(e) => {
									e.preventDefault();
									void handleNotificationToggle();
								}}
								disabled={webPush.loading}
							>
								{webPush.loading ? (
									<Spinner size="sm" loading className="size-icon-sm" />
								) : webPush.subscribed ? (
									<BellIcon className="size-icon-sm" />
								) : (
									<BellOffIcon className="size-icon-sm" />
								)}
								{webPush.subscribed
									? "Turn notifications off"
									: "Turn notifications on"}
							</DropdownMenuItem>
						)}
					</DropdownMenuContent>
				</DropdownMenu>
			)}
		</div>
	);
};
