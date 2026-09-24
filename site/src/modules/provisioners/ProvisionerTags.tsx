import { cn } from "cn";
import { Badge } from "#/components/Badge/Badge";

export const ProvisionerTags: React.FC<React.HTMLProps<HTMLDivElement>> = ({
	className,
	...props
}) => {
	return (
		<div
			{...props}
			className={cn(["flex items-center gap-1 flex-wrap py-0.5", className])}
		/>
	);
};

type ProvisionerTagProps = {
	label: string;
	value?: string;
};

export const ProvisionerTag: React.FC<ProvisionerTagProps> = ({
	label,
	value,
}) => {
	return (
		<Badge className="whitespace-nowrap">
			[{label}
			{value && `=${value}`}]
		</Badge>
	);
};

type ProvisionerTagsProps = {
	tags: Record<string, string>;
};

export const ProvisionerTruncateTags: React.FC<ProvisionerTagsProps> = ({
	tags,
}) => {
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
			{remainderCount > 0 && <Badge>+{remainderCount}</Badge>}
		</ProvisionerTags>
	);
};
