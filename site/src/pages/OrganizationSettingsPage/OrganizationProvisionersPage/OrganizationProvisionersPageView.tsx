import type { ProvisionerDaemon } from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Link } from "components/Link/Link";
import { Loader } from "components/Loader/Loader";
import { Paywall } from "components/Paywall/Paywall";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { SquareArrowOutUpRightIcon, XIcon } from "lucide-react";
import type { FC } from "react";
import { docs } from "utils/docs";
import { LastConnectionHead } from "./LastConnectionHead";
import { ProvisionerRow } from "./ProvisionerRow";

type ProvisionersFilter = {
	ids: string;
};

interface OrganizationProvisionersPageViewProps {
	showPaywall: boolean | undefined;
	provisioners: readonly ProvisionerDaemon[] | undefined;
	buildVersion: string | undefined;
	error: unknown;
	filter: ProvisionersFilter;
	onRetry: () => void;
	onFilterChange: (filter: ProvisionersFilter) => void;
}

export const OrganizationProvisionersPageView: FC<
	OrganizationProvisionersPageViewProps
> = ({
	showPaywall,
	error,
	provisioners,
	buildVersion,
	filter,
	onFilterChange,
	onRetry,
}) => {
	return (
		<section>
			<SettingsHeader>
				<SettingsHeaderTitle>Provisioners</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Coder server runs provisioner daemons which execute terraform during
					workspace and template builds.{" "}
					<Link href={docs("/admin/provisioners")}>View docs</Link>
				</SettingsHeaderDescription>
			</SettingsHeader>

			{filter.ids && (
				<div className="flex items-center gap-2 mb-6">
					<div className="relative">
						<Badge className="h-10 text-sm pl-3 pr-10 font-mono">
							{filter.ids}
						</Badge>
						<div className="size-10 flex items-center justify-center absolute top-0 right-0">
							<TooltipProvider>
								<Tooltip>
									<TooltipTrigger asChild>
										<Button
											size="icon"
											variant="subtle"
											onClick={() => {
												onFilterChange({ ...filter, ids: "" });
											}}
										>
											<span className="sr-only">Clear ID</span>
											<XIcon />
										</Button>
									</TooltipTrigger>
									<TooltipContent>Clear ID</TooltipContent>
								</Tooltip>
							</TooltipProvider>
						</div>
					</div>
				</div>
			)}

			{showPaywall ? (
				<Paywall
					message="Provisioners"
					description="Provisioners run your Terraform to create templates and workspaces. You need a Premium license to use this feature for multiple organizations."
					documentationLink={docs("/")}
				/>
			) : (
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Key</TableHead>
							<TableHead>Version</TableHead>
							<TableHead>Status</TableHead>
							<TableHead>Tags</TableHead>
							<TableHead>
								<LastConnectionHead />
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{provisioners ? (
							provisioners.length > 0 ? (
								provisioners.map((provisioner) => (
									<ProvisionerRow
										provisioner={provisioner}
										key={provisioner.id}
										buildVersion={buildVersion}
										defaultIsOpen={filter.ids.includes(provisioner.id)}
									/>
								))
							) : (
								<TableRow>
									<TableCell colSpan={999}>
										<EmptyState
											message="No provisioners found"
											description="A provisioner is required before you can create templates and workspaces. You can connect your first provisioner by following our documentation."
											cta={
												<Button size="sm" asChild>
													<a href={docs("/admin/provisioners")}>
														Create a provisioner
														<SquareArrowOutUpRightIcon />
													</a>
												</Button>
											}
										/>
									</TableCell>
								</TableRow>
							)
						) : error ? (
							<TableRow>
								<TableCell colSpan={999}>
									<EmptyState
										message="Error loading the provisioner jobs"
										cta={
											<Button onClick={onRetry} size="sm">
												Retry
											</Button>
										}
									/>
								</TableCell>
							</TableRow>
						) : (
							<TableRow>
								<TableCell colSpan={999}>
									<Loader />
								</TableCell>
							</TableRow>
						)}
					</TableBody>
				</Table>
			)}
		</section>
	);
};
