import { LayersIcon, PlusIcon } from "lucide-react";
import { useQueries } from "react-query";
import { Link, useParams } from "react-router";
import {
	foundationPath,
	foundationPermissions,
	foundations,
} from "#/api/queries/foundations";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";

const FoundationsPage: React.FC = () => {
	const { organization } = useParams();
	const { experiments, organizations } = useDashboard();
	const enabled = experiments.includes("template-resources");
	const scopedOrganizations = organization
		? organizations.filter(
				(item) => item.id === organization || item.name === organization,
			)
		: organizations;
	const queries = useQueries({
		queries: scopedOrganizations.map((item) => ({
			...foundations(item.id),
			enabled,
		})),
	});
	const permissions = useQueries({
		queries: scopedOrganizations.map((item) => ({
			...foundationPermissions(item.id),
			enabled,
		})),
	});
	const canCreate = permissions.some(
		(query) => query.data?.canCreateFoundation,
	);
	const isLoading = queries.some((query) => query.isLoading);
	const items = queries.flatMap((query) => query.data ?? []);
	const errors = [...queries, ...permissions].filter((query) => query.error);
	const createHref =
		organization && scopedOrganizations[0]
			? `/foundations/new?organization=${encodeURIComponent(scopedOrganizations[0].id)}`
			: "/foundations/new";

	return (
		<Margins className="pb-12">
			<title>{pageTitle("Foundations")}</title>
			<PageHeader
				actions={
					enabled &&
					canCreate && (
						<Button asChild size="lg">
							<Link to={createHref}>
								<PlusIcon />
								New Foundation
							</Link>
						</Button>
					)
				}
			>
				<PageHeaderTitle>Foundations</PageHeaderTitle>
				<PageHeaderSubtitle>
					Manage shared Terraform infrastructure independently of workspace
					templates.
				</PageHeaderSubtitle>
			</PageHeader>
			{!enabled ? (
				<ErrorAlert error="Enable the template-resources experiment to manage Foundations." />
			) : organization && scopedOrganizations.length === 0 ? (
				<ErrorAlert error="Organization not found or not accessible." />
			) : (
				<>
					{errors.map((query, index) => (
						<div key={index} className="mb-4 space-y-2">
							<ErrorAlert error={query.error} />
							<Button variant="outline" onClick={() => void query.refetch()}>
								Retry Foundations
							</Button>
						</div>
					))}
					<Table aria-label="Foundations">
						<TableHeader>
							<TableRow>
								<TableHead className="w-[40%]">Name</TableHead>
								<TableHead>Organization</TableHead>
								<TableHead>Status</TableHead>
								<TableHead>Template</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{items.map((item) => {
								const owner = organizations.find(
									(org) => org.id === item.organization_id,
								);
								return (
									<TableRow key={item.id}>
										<TableCell>
											<AvatarData
												avatar={
													<Avatar variant="icon">
														<LayersIcon />
													</Avatar>
												}
												title={
													<Link
														className="text-content-primary hover:underline"
														to={foundationPath(item.organization_id, item.id)}
													>
														{item.name || "Untitled Foundation"}
													</Link>
												}
												subtitle={item.id}
											/>
										</TableCell>
										<TableCell>
											{owner?.display_name || owner?.name || "Unknown"}
										</TableCell>
										<TableCell>
											<Badge>{item.status}</Badge>
										</TableCell>
										<TableCell className="text-content-secondary">
											{item.template_id ? "Attached" : "Unattached"}
										</TableCell>
									</TableRow>
								);
							})}
							{isLoading && <TableLoader />}
							{!isLoading && items.length === 0 && errors.length === 0 && (
								<TableRow>
									<TableCell colSpan={4}>
										<EmptyState
											message="No Foundations yet"
											description="Create a Foundation to manage shared infrastructure, then attach it to a template when you are ready."
										/>
									</TableCell>
								</TableRow>
							)}
						</TableBody>
					</Table>
				</>
			)}
		</Margins>
	);
};
export default FoundationsPage;
