import Link from "@mui/material/Link";
import type { TemplateExample } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Pill } from "components/Pill/Pill";
import type { FC, HTMLAttributes } from "react";
import { Link as RouterLink } from "react-router";
import { cn } from "utils/cn";

type TemplateExampleCardProps = HTMLAttributes<HTMLDivElement> & {
	example: TemplateExample;
	activeTag?: string;
};

export const TemplateExampleCard: FC<TemplateExampleCardProps> = ({
	example,
	activeTag,
	...divProps
}) => {
	return (
		<div {...divProps} className={cn(classNames.card, divProps.className)}>
			<div className={classNames.header}>
				<div className={classNames.icon}>
					<ExternalImage
						src={example.icon}
						className="w-full h-full object-contain"
					/>
				</div>

				<div className={classNames.tags}>
					{example.tags.map((tag) => (
						<RouterLink key={tag} to={`/starter-templates?tag=${tag}`}>
							<Pill
								className={cn([
									classNames.tag,
									activeTag === tag && classNames.activeTag,
								])}
							>
								{tag}
							</Pill>
						</RouterLink>
					))}
				</div>
			</div>

			<div>
				<h4 className="text-sm font-semibold m-0 mb-1">{example.name}</h4>
				<span className={classNames.description}>
					{example.description}{" "}
					<Link
						component={RouterLink}
						to={`/starter-templates/${example.id}`}
						className="inline-block text-[13px] leading-none mt-2"
					>
						Read more
					</Link>
				</span>
			</div>

			<div className={classNames.useButtonContainer}>
				<Button asChild className="w-full">
					<RouterLink to={`/templates/new?exampleId=${example.id}`}>
						Use template
					</RouterLink>
				</Button>
			</div>
		</div>
	);
};

const classNames = {
	card: "w-[320px] p-6 rounded-md border border-solid border-border dark:border-surface-quaternary text-left text-inherit flex flex-col",
	header: "flex items-center justify-between mb-6",
	icon: "flex-shrink-0 pt-1 w-8 h-8",
	tags: "flex flex-wrap gap-2 justify-end",
	tag: "border border-solid border-border dark:border-surface-quaternary text-decoration-none cursor-pointer hover:border-sky-500",
	activeTag: "bg-sky-950 border-sky-500 dark:border-sky-500",
	description: "text-[13px] leading-relaxed block text-content-secondary",
	useButtonContainer: "flex flex-col items-center gap-3 pt-6 mt-auto",
};
