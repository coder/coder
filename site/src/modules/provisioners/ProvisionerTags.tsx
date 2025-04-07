import { Badge } from "components/Badge/Badge";
import type { FC, HTMLProps } from "react";
import { cn } from "utils/cn";

export const ProvisionerTags: FC<HTMLProps<HTMLDivElement>> = ({
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

type ProvisionerTagProps = {
	label: string;
	value?: string;
};

export const ProvisionerTag: FC<ProvisionerTagProps> = ({ label, value }) => {
	return (
		<Badge size="sm" className="whitespace-nowrap">
			[{label}
			{value && `=${value}`}]
		</Badge>
	);
};

type ProvisionerTagsProps = {
	tags: Record<string, string>;
};

export const ProvisionerTruncateTags: FC<ProvisionerTagsProps> = ({ tags }) => {
	const keys = Object.keys(tags);

	if (keys.length === 0) {
		return null;
	}

	const firstKey = keys[0];
	const firstValue = tags[firstKey];
	const remainderCount = keys.length - 1;

	return (
		<ProvisionerTags>
			<ProvisionerTag label={firstKey} value={firstValue} />
			{remainderCount > 0 && <Badge size="sm">+{remainderCount}</Badge>}
		</ProvisionerTags>
	);
};
