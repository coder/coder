import type { ProvisionerDaemon } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Link } from "components/Link/Link";
import { Loader } from "components/Loader/Loader";
import { Paywall } from "components/Paywall/Paywall";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { SquareArrowOutUpRightIcon } from "lucide-react";
import type { FC } from "react";
import { docs } from "utils/docs";
import { ProvisionerRow } from "./ProvisionerRow";
import { LastConnectionHead } from "./LastConnectionHead";

interface OrganizationProvisionersPageViewProps {
	showPaywall: boolean | undefined;
	provisioners: readonly ProvisionerDaemon[] | undefined;
	buildVersion: string | undefined;
	error: unknown;
	onRetry: () => void;
}

export const OrganizationProvisionersPageView: FC<
	OrganizationProvisionersPageViewProps
> = ({ showPaywall, error, provisioners, buildVersion, onRetry }) => {
	return (
		<section className="flex flex-col gap-8">
			<header className="flex flex-row items-baseline justify-between">
				<div className="flex flex-col gap-2">
					<h1 className="text-3xl m-0">Provisioners</h1>
					<p className="text-sm text-content-secondary m-0">
						Coder server runs provisioner daemons which execute terraform during
						workspace and template builds.{" "}
						<Link href={docs("/admin/provisioners")}>View docs</Link>
					</p>
				</div>
			</header>

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
