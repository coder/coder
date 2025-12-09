import type { Interpolation, Theme } from "@emotion/react";
import type { TemplateExample } from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { TemplateExampleCard } from "modules/templates/TemplateExampleCard/TemplateExampleCard";
import type { FC } from "react";
import { Link, useSearchParams } from "react-router";
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
					<h2 css={styles.sectionTitle}>Choose a starter template</h2>
					<span css={styles.filterCaption}>Filter</span>
					{tags.map((tag) => (
						<Link
							key={tag}
							to={`?tag=${tag}`}
							css={[styles.tagLink, tag === activeTag && styles.tagLinkActive]}
						>
							{getTagLabel(tag)} ({starterTemplatesByTag[tag].length})
						</Link>
					))}
				</Stack>
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
		</Stack>
	);
};

const styles = {
	filterCaption: (theme) => ({
		textTransform: "uppercase",
		fontWeight: 600,
		fontSize: 12,
		color: theme.palette.text.secondary,
		letterSpacing: "0.1em",
	}),

	tagLink: (theme) => ({
		color: theme.palette.text.secondary,
		textDecoration: "none",
		fontSize: 14,
		textTransform: "capitalize",

		"&:hover": {
			color: theme.palette.text.primary,
		},
	}),

	tagLinkActive: (theme) => ({
		color: theme.palette.text.primary,
		fontWeight: 600,
	}),

	sectionTitle: (theme) => ({
		color: theme.palette.text.primary,
		fontSize: 16,
		fontWeight: 400,
		margin: 0,
	}),
} satisfies Record<string, Interpolation<Theme>>;
