import { Avatar } from "#/components/Avatar/Avatar";

type AvatarGroupItem = {
	readonly id: string;
	readonly name: string;
	readonly src?: string;
};

type AvatarGroupProps = {
	readonly items: readonly AvatarGroupItem[];
	/** Names the group, for example "Shared with 3 users and groups". */
	readonly label: string;
};

const maxVisible = 3;

/** Overlapping avatars with a "+N" count for the rest. */
export const AvatarGroup: React.FC<AvatarGroupProps> = ({ items, label }) => {
	const shown = items.slice(0, maxVisible);
	const hidden = items.length - shown.length;
	return (
		<div className="flex items-center" role="group" aria-label={label}>
			{shown.map((item) => (
				<Avatar
					key={item.id}
					src={item.src}
					fallback={item.name}
					role="img"
					aria-label={item.name}
					title={item.name}
					className="-ml-2 border-surface-primary first:ml-0"
				/>
			))}
			{hidden > 0 && (
				<span
					title={items
						.slice(maxVisible)
						.map((item) => item.name)
						.join(", ")}
					className="-ml-2 flex size-(--avatar-default) items-center justify-center rounded border border-solid border-surface-primary bg-surface-tertiary text-2xs text-content-secondary"
				>
					+{hidden}
				</span>
			)}
		</div>
	);
};
