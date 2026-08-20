import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { type DescribedScope, groupScopes } from "./scopes";

type ScopeListProps = {
	scopes: readonly string[];
};

/**
 * A destructive or undescribed permission carries a label as well as a
 * description — colour alone is never the signal, and "delete" is the thing a
 * user most needs to notice before approving.
 */
const ScopeBadge: FC<{ scope: DescribedScope }> = ({ scope }) => {
	if (!scope.known) {
		return (
			<Badge size="xs" variant="warning">
				Unrecognized
			</Badge>
		);
	}
	if (scope.risk === "destructive" || scope.risk === "unrestricted") {
		return (
			<Badge size="xs" variant="destructive">
				Can delete
			</Badge>
		);
	}
	return null;
};

/**
 * The permissions being granted, grouped by resource. Raw scope strings are
 * kept visible under each description: the description is what the decision is
 * made on, the identifier is what gets debugged later.
 */
export const ScopeList: FC<ScopeListProps> = ({ scopes }) => {
	const groups = groupScopes(scopes);

	const list = (
		<div className="flex flex-col gap-4">
			{groups.map((group) => (
				<div key={group.title} className="flex flex-col gap-2">
					<span className="text-xs text-content-secondary">{group.title}</span>
					<ul className="m-0 flex list-none flex-col gap-2 p-0">
						{group.scopes.map((scope) => (
							<li key={scope.name} className="flex flex-col gap-0.5">
								<span className="flex items-center gap-2 text-sm text-content-primary">
									{scope.description}
									<ScopeBadge scope={scope} />
								</span>
								{/* Mono because the scope name is an identifier. */}
								<span className="font-mono text-2xs text-content-secondary">
									{scope.name}
								</span>
							</li>
						))}
					</ul>
				</div>
			))}
		</div>
	);

	/*
	 * A long request would otherwise push the Approve and Deny buttons below the
	 * fold, which is how someone ends up approving without reading. The list
	 * scrolls instead; the decision stays on screen. `viewportTabIndex` keeps the
	 * scrollable region reachable by keyboard.
	 */
	if (scopes.length > 8) {
		return (
			<ScrollArea className="max-h-64" viewportTabIndex={0}>
				{list}
			</ScrollArea>
		);
	}

	return list;
};
