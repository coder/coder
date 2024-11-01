import { Button } from "@/components/ui/button";
import { ExternalLinkIcon } from "@radix-ui/react-icons";
import type { FC } from "react";
import { docs } from "utils/docs";

export type PremiumPageViewProps = { isEnterprise: boolean };

export const PremiumPageView: FC<PremiumPageViewProps> = ({ isEnterprise }) => {
	return isEnterprise ? <EnterpriseVersion /> : <OSSVersion />;
};

const EnterpriseVersion: FC = () => {
	return (
		<div className="max-w-4xl">
			<header className="flex flex-row justify-between align-baseline pb-5">
				<div>
					<h1 className="text-3xl m-0 font-semibold">Premium</h1>
					<p className="text-sm max-w-xl mt-2 text-content-secondary font-medium">
						As an Enterprise license holder, you already benefit from Coder’s
						features for secure, large-scale deployments. Upgrade to Coder
						Premium for enhanced multi-tenant control and flexibility:
					</p>
				</div>
				<Button asChild>
					<a href="https://coder.com/contact/sales" className="no-underline">
						Upgrade
					</a>
				</Button>
			</header>

			<section className="pb-1">
				<a
					className="no-underline text-sm text-content-link"
					href={docs("/admin/users/organizations")}
				>
					<span className="flex items-center">
						<h2 className="text-sm font-semibold m-0">
							Multi-Organization Access Controls&nbsp;
						</h2>
						<ExternalLinkIcon />
					</span>
				</a>
				<p className="text-sm max-w-xl text-content-secondary mt-0 font-medium">
					Manage multiple teams and projects within a single deployment, each
					with isolated access.
				</p>
			</section>

			<section className="pb-1">
				<a
					className="no-underline text-sm text-content-link"
					href={docs("/admin/users/groups-roles")}
				>
					<span className="flex items-center">
						<h2 className="text-sm font-semibold m-0">Custom Role&nbsp;</h2>
						<ExternalLinkIcon />
					</span>
				</a>
				<p className="text-sm max-w-xl text-content-secondary mt-0 font-medium">
					Configure specific permissions for teams or contractors with tailored
					roles.
				</p>
			</section>

			<section>
				<a
					className="no-underline text-sm text-content-link"
					href={docs("/admin/users/quotas")}
				>
					<span className="flex items-center">
						<h2 className="text-sm font-semibold m-0">
							Org-Level Quotas for Chargeback&nbsp;
						</h2>
						<ExternalLinkIcon />
					</span>
				</a>
				<p className="text-sm max-w-xl text-content-secondary mt-0 font-medium">
					Set and monitor resource quotas at the organization level to support
					internal cost tracking.
				</p>
			</section>

			<section className="pt-10">
				<p className="text-sm max-w-xl text-content-secondary mt-0 font-medium">
					These Premium features enable you to manage team structure and budget
					allocation across multiple platform teams.
				</p>
			</section>
		</div>
	);
};

const OSSVersion: FC = () => {
	return (
		<div className="max-w-4xl">
			<div className="flex flex-row justify-between align-baseline pb-10">
				<div>
					<h1 className="text-3xl m-0 text-content-primary font-semibold">
						Premium
					</h1>
					<p className="text-sm max-w-xl mt-2 text-content-secondary">
						Coder Premium is designed for enterprises that need to scale their
						Coder deployment efficiently, securely, and with full control. By
						upgrading, your team gains access to advanced features enabling
						governance across all environments.
					</p>
				</div>
				<Button asChild>
					<a
						href="https://coder.com/pricing#compare-plans"
						className="no-underline"
					>
						Compare Plans
					</a>
				</Button>
			</div>

			<section className="pb-10 max-w-xl text-sm text-content-secondary">
				<h2 className="text-xl text-content-primary m-0">
					Deploy Coder at Scale
				</h2>
				<p>
					Equip your enterprise to deploy and manage thousands of workspaces
					with performance and reliability.
				</p>
				<ul className="pl-5">
					<li>
						<span className="text-content-primary font-semibold">
							High Availability
						</span>
						<br />
						<span className="font-medium">
							Scale with automatic failover and load balancing across multiple
							Coder instances.
						</span>
					</li>
					<li>
						<span className="text-content-primary font-semibold">
							Multi-Organization Access Control
						</span>
						<br />
						<span className="font-medium">
							Isolate teams, projects, and environments within a single Coder
							deployment.
						</span>
					</li>
					<li>
						<span className="text-content-primary font-semibold">
							Unlimited External Authentication Integrations
						</span>
						<br />
						<span className="font-medium">
							Integrate with external service providers like GitHub, JFrog, and
							Vault.
						</span>
					</li>
				</ul>
			</section>

			<section className="pb-10 max-w-xl text-sm text-content-secondary">
				<h2 className="text-xl text-content-primary m-0">
					Control Infrastructure Costs
				</h2>
				<p>
					Optimize cloud usage and maintain cost-effective resource management
					for your teams.
				</p>
				<ul className="pl-5">
					<li>
						<span className="text-content-primary font-semibold">
							Auto-Stop Idle Workspaces
						</span>
						<br />
						<span className="font-medium">
							Automatically shut down inactive workspaces to prevent unnecessary
							costs.
						</span>
					</li>
					<li>
						<span className="text-content-primary font-semibold">
							Resource Quotas
						</span>
						<br />
						<span className="font-medium">
							Set user and team-specific limits to control spending and resource
							allocation.
						</span>
					</li>
					<li>
						<span className="text-content-primary font-semibold">
							Usage Insights
						</span>
						<br />
						<span className="font-medium">
							Track workspace usage patterns to make data-driven decisions about
							infrastructure needs.
						</span>
					</li>
				</ul>
			</section>

			<section className="pb-5 max-w-xl text-sm text-content-secondary">
				<h2 className="text-xl text-content-primary m-0">
					Govern Workspace Activity
				</h2>
				<p>
					Maintain security and compliance across your organization with robust
					governance features.
				</p>
				<ul className="pl-5">
					<li>
						<span className="text-content-primary font-semibold">
							Audit Logging
						</span>
						<br />
						<span className="font-medium">
							Capture detailed records of user actions and workspace activity
							for compliance and security.
						</span>
					</li>
					<li>
						<span className="text-content-primary font-semibold">
							Template Permissions
						</span>
						<br />
						<span className=" font-medium">
							Control who can create, modify, and access workspace templates
							across teams.
						</span>
					</li>
					<li>
						<span className="text-content-primary font-semibold">
							Workspace Command Logging
						</span>
						<br />
						<span className="font-medium">
							Monitor and log critical commands to ensure security and
							compliance standards are met.
						</span>
					</li>
				</ul>
			</section>
		</div>
	);
};
