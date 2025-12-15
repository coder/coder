import type { TemplateExample } from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { TemplateExampleCard } from "modules/templates/TemplateExampleCard/TemplateExampleCard";
import type { FC } from "react";
import { Link, useSearchParams } from "react-router";
import { cn } from "utils/cn";
import type { StarterTemplatesByTag } from "utils/starterTemplates";

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
	// The tasks-docker template should be first, as it's the easiest way to
	// get started with Coder. The docker template should be second.
	const featuredTemplateIds = ["tasks-docker", "docker"];

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
		<Stack direction="row" spacing={4} alignItems="flex-start">
			{starterTemplatesByTag && tags && (
				<Stack className="w-[202px] flex-shrink-0 sticky">
					<h2 className={classNames.sectionTitle}>Choose a starter template</h2>
					<span className={classNames.filterCaption}>Filter</span>
					{tags.map((tag) => (
						<Link
							key={tag}
							to={`?tag=${tag}`}
							className={cn(
								classNames.tagLink,
								tag === activeTag && classNames.tagLinkActive,
							)}
						>
							{getTagLabel(tag)} ({starterTemplatesByTag[tag].length})
						</Link>
					))}
				</Stack>
			)}

			<div className="flex flex-wrap gap-8 h-max">
				{visibleTemplates?.map((example) => (
					<TemplateExampleCard
						className="bg-surface-secondary leading-"
						example={example}
						key={example.id}
						activeTag={activeTag}
					/>
				))}
			</div>
		</Stack>
	);
};

const classNames = {
	filterCaption:
		"uppercase font-semibold text-xs text-content-secondary tracking-[0.1em] leading-loose",
	tagLink:
		"text-content-secondary no-underline text-sm capitalize hover:text-content-primary",
	tagLinkActive: "text-content-primary font-semibold",
	sectionTitle: "text-content-primary text-base font-normal m-0",
};
