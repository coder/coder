import { CircleCheckIcon, CircleMinusIcon, TagIcon, XIcon } from "lucide-react";
import type { ComponentProps, FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";

const parseBool = (s: string): { valid: boolean; value: boolean } => {
	switch (s.toLowerCase()) {
		case "true":
		case "yes":
		case "1":
			return { valid: true, value: true };
		case "false":
		case "no":
		case "0":
		case "":
			return { valid: true, value: false };
		default:
			return { valid: false, value: false };
	}
};

interface ProvisionerTagProps {
	tagName: string;
	tagValue: string;
	/** Only used in the TemplateVersionEditor */
	onDelete?: (tagName: string) => void;
}

export const ProvisionerTag: FC<ProvisionerTagProps> = ({
	tagName,
	tagValue,
	onDelete,
}) => {
	const { valid, value: boolValue } = parseBool(tagValue);
	const kv = (
		<>
			<span className="font-semibold">{tagName}</span> <span>{tagValue}</span>
		</>
	);
	const content = onDelete ? (
		<>
			{kv}
			<Button
				size="icon"
				variant="subtle"
				onClick={() => {
					onDelete(tagName);
				}}
				className="size-6 -my-1"
			>
				<XIcon className="size-icon-xs" />
				<span className="sr-only">Delete {tagName}</span>
			</Button>
		</>
	) : (
		kv
	);
	if (valid) {
		return <BooleanPill value={boolValue}>{content}</BooleanPill>;
	}
	return (
		<Badge variant="outline" size="md" data-testid={`tag-${tagName}`}>
			<TagIcon className="size-icon-sm" />
			{content}
		</Badge>
	);
};

type BooleanPillProps = Omit<
	ComponentProps<typeof Badge>,
	"variant" | "value"
> & {
	value: boolean;
};

const BooleanPill: FC<BooleanPillProps> = ({
	value,
	children,
	...badgeProps
}) => {
	return (
		<Badge variant={value ? "info" : "warning"} size="md" {...badgeProps}>
			{value ? (
				<CircleCheckIcon className="size-icon-sm text-content-link" />
			) : (
				<CircleMinusIcon className="size-icon-sm text-content-warning" />
			)}
			{children}
		</Badge>
	);
};
