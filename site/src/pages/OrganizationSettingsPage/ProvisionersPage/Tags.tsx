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

type TruncateTagsProps = {
	tags: Record<string, string>;
};

export const TruncateTags: FC<TruncateTagsProps> = ({ tags }) => {
	const keys = Object.keys(tags);

	if (keys.length === 0) {
		return null;
	}

	const firstKey = keys[0];
	const firstValue = tags[firstKey];
	const remainderCount = keys.length - 1;

	return (
		<Tags>
			<Tag label={firstKey} value={firstValue} />
			{remainderCount > 0 && <Badge size="sm">+{remainderCount}</Badge>}
		</Tags>
	);
};
