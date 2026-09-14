import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import { Button } from "#/components/Button/Button";

interface AlternativeLink {
	label: string;
	href: string;
	external: boolean;
}

const alternatives: readonly AlternativeLink[] = [
	{
		label: "Start from scratch",
		href: "https://coder.com/docs/tutorials/template-from-scratch",
		external: true,
	},
	{
		label: "Upload an existing template",
		href: "/templates/new",
		external: false,
	},
	{
		label: "Browse community templates",
		href: "https://registry.coder.com/templates",
		external: true,
	},
	{
		label: "Use template agent skill",
		href: "https://registry.coder.com/skills",
		external: true,
	},
];

export const TemplateAlternatives: FC = () => {
	return (
		<div className="p-6 border border-solid rounded-lg mt-6">
			<p className="text-sm text-content-secondary mb-4 mt-0">
				Alternatives to create a template:
			</p>
			<div className="flex flex-wrap gap-2">
				{alternatives.map((alt) =>
					alt.external ? (
						<Button key={alt.label} variant="outline" size="sm" asChild>
							<a href={alt.href} target="_blank" rel="noreferrer">
								{alt.label}
								<ExternalLinkIcon />
							</a>
						</Button>
					) : (
						<Button key={alt.label} variant="outline" size="sm" asChild>
							<Link to={alt.href}>{alt.label}</Link>
						</Button>
					),
				)}
			</div>
		</div>
	);
};
