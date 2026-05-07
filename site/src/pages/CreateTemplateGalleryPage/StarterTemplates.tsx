import type { FC } from "react";
import { Link, useSearchParams } from "react-router";
import type { TemplateExample } from "#/api/typesGenerated";
import { TemplateExampleCard } from "#/modules/templates/TemplateExampleCard/TemplateExampleCard";
import { cn } from "#/utils/cn";
import type { StarterTemplatesByTag } from "#/utils/starterTemplates";

const getTagLabel = (tag: string) => {
	const labelByTag: Record<string, string> = {
		all: "All templates",
		digitalocean: "DigitalOcean",
		aws: "AWS",
		google: "Google Cloud",
	};
	return labelByTag[tag] ?? tag;
};

const selectTags = (starterTemplatesByTag: StarterTemplatesByTag) => {
	return starterTemplatesByTag
		? Object.keys(starterTemplatesByTag).sort((a, b) => a.localeCompare(b))
		: undefined;
};

const sortVisibleTemplates = (templates: TemplateExample[]) => {
	// The quickstart template should be first, as it's the easiest
	// way to get started with Coder.
	const featuredTemplateIds = ["quickstart", "docker"];

	const featuredTemplates: TemplateExample[] = [];
	for (const id of featuredTemplateIds) {
		for (const template of templates) {
			if (id === template.id) {
				featuredTemplates.push(template);
			}
		}
	}

	const nonFeaturedTemplates = templates
		.filter((template) => {
			return !featuredTemplateIds.includes(template.id);
		})
		.sort((a, b) => {
			return a.name.localeCompare(b.name);
		});

	return [...featuredTemplates, ...nonFeaturedTemplates];
};

interface StarterTemplatesProps {
	starterTemplatesByTag?: StarterTemplatesByTag;
}

export const StarterTemplates: FC<StarterTemplatesProps> = ({
	starterTemplatesByTag,
}) => {
	const [urlParams] = useSearchParams();
	const tags = starterTemplatesByTag
		? selectTags(starterTemplatesByTag)
		: undefined;
	const activeTag = urlParams.get("tag") ?? "all";
	const visibleTemplates = starterTemplatesByTag
		? sortVisibleTemplates(starterTemplatesByTag[activeTag])
		: undefined;

	return (
		<div className="flex flex-row gap-8 items-start">
			{starterTemplatesByTag && tags && (
				<div className="flex flex-col gap-4 w-[202px] shrink-0 sticky">
					<h2 className="m-0 text-base font-normal text-content-primary">
						Choose a starter template
					</h2>
					<span className="text-xs font-semibold uppercase tracking-[0.1em] text-content-secondary">
						Filter
					</span>
					{tags.map((tag) => (
						<Link
							key={tag}
							to={`?tag=${tag}`}
							className={cn(
								"text-sm capitalize no-underline hover:text-content-primary",
								tag === activeTag
									? "font-semibold text-content-primary"
									: "text-content-secondary",
							)}
						>
							{getTagLabel(tag)} ({starterTemplatesByTag[tag].length})
						</Link>
					))}
				</div>
			)}

			<div className="flex flex-wrap gap-8 h-max">
				{visibleTemplates?.map((example) => (
					<TemplateExampleCard
						css={(theme) => ({
							backgroundColor: theme.palette.background.paper,
						})}
						example={example}
						key={example.id}
						activeTag={activeTag}
					/>
				))}
			</div>
		</div>
	);
};
