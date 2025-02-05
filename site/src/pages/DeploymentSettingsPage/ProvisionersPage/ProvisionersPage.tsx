import { AvatarFallback } from "@radix-ui/react-avatar";
import type { ProvisionerJob } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { Link } from "components/Link/Link";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import { BanIcon } from "lucide-react";
import type { FC } from "react";
import { docs } from "utils/docs";

export const ProvisionersPage: FC = () => {
	return (
		<div className="flex flex-col gap-12">
			<header className="flex flex-row items-baseline justify-between">
				<div className="flex flex-col gap-2">
					<h1 className="text-3xl m-0">Provisioners</h1>
				</div>
			</header>

			<main>
				<Tabs active="jobs">
					<TabsList>
						<TabLink value="jobs" to="?tab=jobs">
							Jobs
						</TabLink>
						<TabLink value="daemons" to="?tab=daemons">
							Daemons
						</TabLink>
					</TabsList>
				</Tabs>

				<div className="mt-6">
					<JobsTabContent />
				</div>
			</main>
		</div>
	);
};

const JobsTabContent: FC = () => {
	return (
		<section className="flex flex-col gap-8">
			<p className="text-sm text-content-secondary m-0 mt-2">
				Provisioner Jobs are the individual tasks assigned to Provisioners when
				the workspaces are being built.{" "}
				<Link href={docs("/admin/provisioners")}>View docs</Link>
			</p>

			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Last seen</TableHead>
						<TableHead>Name</TableHead>
						<TableHead>Template</TableHead>
						<TableHead>Tags</TableHead>
						<TableHead>Status</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					<TableRow>
						<TableCell>5 min ago</TableCell>
						<TableCell>
							<Badge size="sm">workspace_build</Badge>
						</TableCell>
						<TableCell>
							<div className="flex items-center gap-1">
								<Avatar fallback="CD" />
								Write Coder on Coder
							</div>
						</TableCell>
						<TableCell>
							<Badge size="sm">[foo=bar]</Badge>
						</TableCell>
						<TableCell>Completed</TableCell>
						<TableCell className="text-right">
							<Button aria-label="Cancel job" size="icon" variant="outline">
								<BanIcon />
							</Button>
						</TableCell>
					</TableRow>
				</TableBody>
			</Table>
		</section>
	);
};

export default ProvisionersPage;
