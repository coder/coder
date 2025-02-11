import { Badge } from "components/Badge/Badge";
import type { FC, HTMLProps } from "react";
import { cn } from "utils/cn";

export const Tags: FC<HTMLProps<HTMLDivElement>> = ({
	className,
	...props
}) => {
	return (
		<div
			{...props}
			className={cn(["flex items-center gap-1 flex-wrap", className])}
		/>
	);
};

type TagProps = {
	label: string;
	value?: string;
};

export const Tag: FC<TagProps> = ({ label, value }) => {
	return (
		<Badge size="sm" className="whitespace-nowrap">
			[{label}
			{value && `=${value}`}]
		</Badge>
	);
};

type TagsProps = {
	tags: Record<string, string>;
};

export const ShrinkTags: FC<TagsProps> = ({ tags }) => {
	const keys = Object.keys(tags);

	if (keys.length === 0) {
		return null;
	}

	const firstKey = keys[0];
	const firstValue = tags[firstKey];
	const restKeys = keys.slice(1);

	return (
		<Tags>
			<Tag label={firstKey} value={firstValue} />
			{restKeys.length > 0 && <Badge size="sm">+{restKeys.length}</Badge>}
		</Tags>
	);
};
