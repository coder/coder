import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import { Button } from "#/components/Button/Button";

interface ExtendStepProps {
	onBack: () => void;
	onFinish: () => void;
}

export const ExtendStep: FC<ExtendStepProps> = ({ onBack, onFinish }) => {
	return (
		<div className="flex min-h-[460px] flex-col gap-4">
			<h2 className="text-2xl font-semibold">Extend Coder Agents</h2>

			<div className="grid grid-cols-1 gap-6 md:grid-cols-3">
				<FeatureCard
					title="Set up instructions"
					description="Control the system prompts and plan mode instructions used across the deployment."
					linkText="Add instructions"
					linkTo="/agents/settings/instructions"
				/>
				<FeatureCard
					title="Add MCP servers"
					description="Configure and extend agent behavior with centrally managed MCP tools, skills, and policies."
					linkText="Configure MCP servers"
					linkTo="/agents/settings/mcp-servers"
				/>
				<FeatureCard
					title="Control templates"
					description="Restrict which templates agents can use to create workspaces."
					linkText="Select templates"
					linkTo="/agents/settings/templates"
				/>
			</div>

			<div className="flex items-center justify-between">
				<Button variant="subtle" className="min-w-0 px-0" onClick={onBack}>
					Back
				</Button>
				<Button onClick={onFinish}>Start chatting</Button>
			</div>
		</div>
	);
};

interface FeatureCardProps {
	title: string;
	description: string;
	linkText: string;
	linkTo: string;
}

const FeatureCard: FC<FeatureCardProps> = ({
	title,
	description,
	linkText,
	linkTo,
}) => {
	return (
		<div className="flex flex-col gap-3">
			{/* Placeholder image */}
			<div className="h-40 w-full overflow-hidden rounded-lg bg-gradient-to-br from-surface-tertiary to-surface-secondary" />
			<h3 className="text-base font-semibold">{title}</h3>
			<p className="text-sm text-content-secondary">{description}</p>
			<Link
				to={linkTo}
				target="_blank"
				className="inline-flex items-center gap-1 text-sm text-content-link transition-colors hover:text-content-link/80"
			>
				{linkText}
				<ExternalLinkIcon className="size-3" />
			</Link>
		</div>
	);
};
