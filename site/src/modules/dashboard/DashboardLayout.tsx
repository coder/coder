import Link from "@mui/material/Link";
import Snackbar from "@mui/material/Snackbar";
import { useAuthenticated } from "hooks";
import { InfoIcon } from "lucide-react";
import { AnnouncementBanners } from "modules/dashboard/AnnouncementBanners/AnnouncementBanners";
import { LicenseBanner } from "modules/dashboard/LicenseBanner/LicenseBanner";
import { type FC, type HTMLAttributes, Suspense } from "react";
import { Outlet } from "react-router";
import { cn } from "utils/cn";
import { docs } from "utils/docs";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { DeploymentBanner } from "./DeploymentBanner/DeploymentBanner";
import { Navbar } from "./Navbar/Navbar";
import { useUpdateCheck } from "./useUpdateCheck";

export const DashboardLayout: FC = () => {
	const { permissions } = useAuthenticated();
	const updateCheck = useUpdateCheck(permissions.viewDeploymentConfig);
	const canViewDeployment = Boolean(permissions.viewDeploymentConfig);

	return (
		<>
			{canViewDeployment && <LicenseBanner />}
			<AnnouncementBanners />

			<div className="flex flex-col min-h-screen justify-between">
				{/* biome-ignore lint/a11y/useValidAnchor: Skip links use fragment anchors by design. */}
				<a
					href="#main-content"
					onClick={(e) => {
						e.preventDefault();
						const main = document.getElementById("main-content");
						main?.focus();
					}}
					className="sr-only focus-visible:not-sr-only focus-visible:absolute focus-visible:z-50 focus-visible:p-4 focus-visible:bg-surface-primary focus-visible:text-content-primary"
				>
					Skip to main content
				</a>
				<Navbar />

				<main
					id="main-content"
					tabIndex={-1}
					className={cn(
						"relative flex flex-col flex-1 min-h-0 overflow-y-auto",
						"focus:outline-none",
					)}
				>
					<Suspense fallback={<Loader />}>
						<Outlet />
					</Suspense>
				</main>

				<DeploymentBanner />

				<Snackbar
					data-testid="update-check-snackbar"
					open={updateCheck.isVisible}
					anchorOrigin={{
						vertical: "bottom",
						horizontal: "right",
					}}
					ContentProps={{
						sx: (theme) => ({
							background: theme.palette.background.paper,
							color: theme.palette.text.primary,
							maxWidth: 440,
							flexDirection: "row",
							borderColor: theme.palette.info.light,

							"& .MuiSnackbarContent-message": {
								flex: 1,
							},

							"& .MuiSnackbarContent-action": {
								marginRight: 0,
							},
						}),
					}}
					message={
						<div css={{ display: "flex", gap: 16 }}>
							<InfoIcon
								className="size-icon-xs"
								css={(theme) => ({
									fontSize: 16,
									height: 20, // 20 is the height of the text line so we can align them
									color: theme.palette.info.light,
								})}
							/>
							<p>
								Coder {updateCheck.data?.version} is now available. View the{" "}
								<Link href={updateCheck.data?.url}>release notes</Link> and{" "}
								<Link href={docs("/install/upgrade")}>
									upgrade instructions
								</Link>{" "}
								for more information.
							</p>
						</div>
					}
					action={
						<Button variant="subtle" size="sm" onClick={updateCheck.dismiss}>
							Dismiss
						</Button>
					}
				/>
			</div>
		</>
	);
};

export const DashboardFullPage: FC<HTMLAttributes<HTMLDivElement>> = ({
	children,
	...attrs
}) => {
	return (
		<div
			{...attrs}
			css={{
				flex: 1,
				display: "flex",
				flexDirection: "column",
				flexBasis: 0,
				minHeight: "100%",
			}}
		>
			{children}
		</div>
	);
};
