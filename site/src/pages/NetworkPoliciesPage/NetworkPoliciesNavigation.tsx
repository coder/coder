import { useQueries } from "react-query";
import { Link } from "react-router";
import { networkPolicyTemplates } from "#/api/queries/networkPolicies";
import type { AuthorizationResponse, Organization } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";

type NetworkPoliciesNavigationProps = {
	organizations: readonly Organization[];
	templateUpdatePermissions: AuthorizationResponse;
};

/** Includes template ACL admins even when they cannot enter organization settings. */
export const NetworkPoliciesNavigation: React.FC<
	NetworkPoliciesNavigationProps
> = ({ organizations, templateUpdatePermissions }) => {
	const queries = useQueries({
		queries: organizations.map((organization) =>
			networkPolicyTemplates(organization.name),
		),
	});
	const available = organizations.filter(
		(organization, index) =>
			templateUpdatePermissions[organization.id] ||
			Boolean(queries[index].data?.length),
	);
	const isLoading = queries.some((query) => query.isLoading);
	if (
		available.length === 0 &&
		!isLoading &&
		queries.every((query) => !query.error)
	)
		return null;
	return (
		<div className="mb-6 space-y-4">
			<Spinner loading={isLoading} label="Loading network policy access" />
			{queries.map((query, index) =>
				query.error ? (
					<div key={organizations[index].id} className="space-y-2">
						<ErrorAlert error={query.error} />
						<Button variant="outline" onClick={() => void query.refetch()}>
							Retry network policies for{" "}
							{organizations[index].display_name || organizations[index].name}
						</Button>
					</div>
				) : null,
			)}
			{available.length > 0 && (
				<nav aria-label="Network policies" className="flex flex-wrap gap-2">
					{available.map((organization) => (
						<Button asChild variant="outline" key={organization.id}>
							<Link
								to={`/organizations/${encodeURIComponent(organization.name)}/network-policies`}
							>
								Network policies
								{organizations.length > 1
									? `: ${organization.display_name || organization.name}`
									: ""}
							</Link>
						</Button>
					))}
				</nav>
			)}
		</div>
	);
};
