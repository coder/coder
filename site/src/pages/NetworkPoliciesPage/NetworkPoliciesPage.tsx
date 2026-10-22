import { useEffect, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link, useLocation, useNavigate, useParams } from "react-router";
import {
	createNetworkPolicy,
	networkPolicies,
	networkPolicy,
	networkPolicyTemplates,
} from "#/api/queries/networkPolicies";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";
import { CreateNetworkPolicyForm } from "./CreateNetworkPolicyForm";
import { NetworkPoliciesPageView } from "./NetworkPoliciesPageView";
import { NetworkPolicyDetail } from "./NetworkPolicyDetail";

type NetworkPoliciesPageProps = { create?: boolean };

/** Gates organization-scoped policy queries and management behind the experiment. */
const NetworkPoliciesPage: React.FC<NetworkPoliciesPageProps> = ({
	create = false,
}) => {
	const { organization = "", policyId = "" } = useParams();
	const { pathname } = useLocation();
	const headingRef = useRef<HTMLHeadingElement>(null);
	// Route changes remove the focused form or link; refetches must not move focus.
	useEffect(() => {
		headingRef.current?.focus();
	}, [pathname]);
	const { experiments, organizations } = useDashboard();
	const enabled = experiments.includes("network-policies");
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const basePath = `/organizations/${encodeURIComponent(organization)}/network-policies`;
	const list = useQuery({
		...networkPolicies(organization),
		enabled: enabled && !create && !policyId,
	});
	const detail = useQuery({
		...networkPolicy(organization, policyId),
		enabled: enabled && Boolean(policyId),
	});
	const templates = useQuery({
		...networkPolicyTemplates(organization),
		enabled: enabled && (create || Boolean(policyId)),
	});
	const createMutation = useMutation(
		createNetworkPolicy(queryClient, organization),
	);
	const error = detail.error || templates.error;
	const organizationLabel =
		organizations.find(
			(org) => org.name === organization || org.id === organization,
		)?.display_name || organization;
	return (
		<Margins className="pb-12">
			<title>{pageTitle("Network policies")}</title>
			<PageHeader
				actions={
					enabled && !create && !policyId ? (
						<Button asChild>
							<Link to={`${basePath}/new`}>Create policy</Link>
						</Button>
					) : undefined
				}
			>
				<PageHeaderTitle ref={headingRef} tabIndex={-1}>
					{create
						? "Create network policy"
						: detail.data?.name || "Network policies"}{" "}
					<Badge>Experimental</Badge>
				</PageHeaderTitle>
				<PageHeaderSubtitle>
					{organizationLabel}:{" "}
					{detail.data?.description ||
						"Manage network policies independently of workspace builds."}
				</PageHeaderSubtitle>
			</PageHeader>
			<div className="mb-6">
				<Link
					className="text-content-link text-sm"
					to={create || policyId ? basePath : "/templates"}
				>
					{create || policyId
						? "Back to network policies"
						: "Back to templates"}
				</Link>
			</div>
			{!enabled ? (
				<p>Enable the network-policies experiment to use this feature.</p>
			) : (
				<>
					{Boolean(error) && (
						<div className="mb-6 space-y-4">
							<ErrorAlert error={error} />
							<Button
								variant="outline"
								onClick={() => {
									if (policyId) void detail.refetch();
									void templates.refetch();
								}}
							>
								Retry
							</Button>
						</div>
					)}
					{!create && !policyId && (
						<NetworkPoliciesPageView
							policies={list.data}
							error={list.error}
							isLoading={list.isLoading}
							onRetry={() => void list.refetch()}
							basePath={basePath}
						/>
					)}
					{(create || policyId) &&
						(templates.isLoading || (policyId && detail.isLoading)) && (
							<Loader />
						)}
					{create && templates.data && (
						<CreateNetworkPolicyForm
							key={organization}
							templates={templates.data}
							isPending={createMutation.isPending}
							error={createMutation.error}
							onSubmit={(request) =>
								createMutation.mutate(request, {
									onSuccess: (policy) => navigate(`${basePath}/${policy.id}`),
								})
							}
						/>
					)}
					{policyId && detail.data && templates.data && (
						<NetworkPolicyDetail
							key={detail.data.id}
							organization={organization}
							policy={detail.data}
							templates={templates.data}
						/>
					)}
				</>
			)}
		</Margins>
	);
};

export default NetworkPoliciesPage;
