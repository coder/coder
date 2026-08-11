import { TriangleAlertIcon, XIcon } from "lucide-react";
import { type FC, useRef } from "react";
import { JobError } from "#/api/queries/templates";
import type { TemplateVersion } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Drawer,
	DrawerClose,
	DrawerContent,
	DrawerTitle,
} from "#/components/Drawer/Drawer";
import { Loader } from "#/components/Loader/Loader";
import { AlertVariant } from "#/modules/provisioners/ProvisionerAlert";
import { ProvisionerStatusAlert } from "#/modules/provisioners/ProvisionerStatusAlert";
import { useWatchVersionLogs } from "#/modules/templates/useWatchVersionLogs";
import { WorkspaceBuildLogs } from "#/modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { navHeight } from "#/theme/constants";

type BuildLogsDrawerProps = {
	error: unknown;
	open: boolean;
	onClose: () => void;
	/**
	 * The element that opened the drawer. Focus returns here on close because,
	 * unlike a `DrawerTrigger`, the opener lives outside the drawer's tree.
	 */
	openerRef?: React.RefObject<HTMLElement | null>;
	templateVersion: TemplateVersion | undefined;
	variablesSectionRef: React.RefObject<HTMLDivElement | null>;
};

export const BuildLogsDrawer: FC<BuildLogsDrawerProps> = ({
	templateVersion,
	error,
	variablesSectionRef,
	open,
	onClose,
	openerRef,
}) => {
	const logs = useWatchVersionLogs(templateVersion);
	// Set when a close should keep focus on the variables input (the "Fill
	// variables" flow) rather than returning it to the opener.
	const preserveVariablesFocusRef = useRef(false);

	const isMissingVariables =
		error instanceof JobError &&
		error.job.error_code === "REQUIRED_TEMPLATE_VARIABLES";

	const matchingProvisioners = templateVersion?.matched_provisioners?.count;
	const availableProvisioners =
		templateVersion?.matched_provisioners?.available;
	const hasLogs = logs && logs.length > 0;

	return (
		<Drawer
			open={open}
			onOpenChange={(isOpen) => {
				if (!isOpen) {
					onClose();
				}
			}}
			direction="right"
		>
			<DrawerContent
				className="!w-[min(800px,100%)] !max-w-full"
				onCloseAutoFocus={(event) => {
					// Radix focuses a DrawerTrigger on close by default, but this drawer
					// has none, so restore focus to the opener (or the variables input
					// for the "Fill variables" flow) to avoid dropping keyboard users on
					// the document body.
					if (preserveVariablesFocusRef.current) {
						preserveVariablesFocusRef.current = false;
						event.preventDefault();
						variablesSectionRef.current?.querySelector("input")?.focus();
						return;
					}
					const opener = openerRef?.current;
					if (opener?.isConnected) {
						event.preventDefault();
						opener.focus();
					}
				}}
			>
				<div className="flex h-full flex-col">
					<header
						className="flex items-center justify-between border-b border-border px-6 bg-surface-secondary"
						style={{ height: navHeight }}
					>
						<DrawerTitle className="m-0 text-base font-medium">
							Creating template...
						</DrawerTitle>
						<DrawerClose asChild>
							<Button size="icon-lg" variant="subtle">
								<XIcon />
								<span className="sr-only">Close build logs</span>
							</Button>
						</DrawerClose>
					</header>

					{isMissingVariables ? (
						<MissingVariablesBanner
							onFillVariables={() => {
								variablesSectionRef.current?.scrollIntoView({
									behavior: "smooth",
								});
								preserveVariablesFocusRef.current = true;
								onClose();
							}}
						/>
					) : (
						<>
							{(matchingProvisioners === 0 || !hasLogs) && (
								<ProvisionerStatusAlert
									matchingProvisioners={matchingProvisioners}
									availableProvisioners={availableProvisioners}
									tags={templateVersion?.job.tags ?? {}}
									variant={AlertVariant.Inline}
								/>
							)}

							{hasLogs ? (
								<section className="flex-1 overflow-auto bg-surface-primary">
									<WorkspaceBuildLogs logs={logs} className="border-0" />
								</section>
							) : (
								<Loader />
							)}
						</>
					)}
				</div>
			</DrawerContent>
		</Drawer>
	);
};

type MissingVariablesBannerProps = { onFillVariables: () => void };

const MissingVariablesBanner: FC<MissingVariablesBannerProps> = ({
	onFillVariables,
}) => {
	return (
		<div className="flex items-center justify-center p-10">
			<div className="flex max-w-[360px] flex-col items-center text-center">
				<TriangleAlertIcon className="size-icon-lg text-content-warning" />
				<h4 className="m-0 mt-4 font-medium leading-none">Missing variables</h4>
				<p className="m-0 mt-2 text-sm leading-6 text-content-secondary">
					During the build process, we identified some missing variables. Rest
					assured, we have automatically added them to the form for you.
				</p>
				<Button
					className="mt-4"
					size="sm"
					variant="outline"
					onClick={onFillVariables}
				>
					Fill variables
				</Button>
			</div>
		</div>
	);
};
