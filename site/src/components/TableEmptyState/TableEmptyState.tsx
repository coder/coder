import { Button } from "#/components/Button/Button";
import { TableCell, TableRow } from "#/components/Table/Table";
import {
	LightbulbGlyph,
	PlusGlyph,
	SearchGlyph,
	SettingsAlertGlyph,
	StarAlertGlyph,
} from "./TableEmptyStateIcons";

/**
 * Why a table has no rows. `docs` means the feature is set up outside this
 * screen; `create` means it is set up from this screen.
 */
type TableEmptyStateVariant =
	| "search"
	| "docs"
	| "create"
	| "upgrade"
	| "default";

const variantIcons: Record<
	TableEmptyStateVariant,
	React.FC<React.SVGProps<SVGSVGElement>>
> = {
	search: SearchGlyph,
	docs: SettingsAlertGlyph,
	create: PlusGlyph,
	upgrade: StarAlertGlyph,
	default: LightbulbGlyph,
};

type SearchTableEmptyStateProps = {
	variant: "search";
	message: string;
	onResetFilters: () => void;
};

type DocsTableEmptyStateProps = {
	variant: "docs";
	message: string;
	description: React.ReactNode;
	docsHref: string;
	docsLabel: string;
};

type CustomTableEmptyStateProps = {
	variant: "create" | "upgrade" | "default";
	message: string;
	description?: React.ReactNode;
	/** Use an outline button unless this is the page's single primary action. */
	cta?: React.ReactNode;
};

type TableEmptyStateProps =
	| SearchTableEmptyStateProps
	| DocsTableEmptyStateProps
	| CustomTableEmptyStateProps;

/**
 * Full-width table row that explains why a table is empty and what to do
 * next. Render it inside a `TableBody`.
 */
export const TableEmptyState: React.FC<TableEmptyStateProps> = (props) => {
	const Icon = variantIcons[props.variant];

	let description: React.ReactNode;
	let cta: React.ReactNode;
	switch (props.variant) {
		case "search":
			description = "Broaden your search to explore more options.";
			cta = (
				<Button variant="outline" size="sm" onClick={props.onResetFilters}>
					Reset filters
				</Button>
			);
			break;
		case "docs":
			description = props.description;
			cta = (
				<Button asChild variant="outline" size="sm">
					<a href={props.docsHref} target="_blank" rel="noreferrer">
						{props.docsLabel}
					</a>
				</Button>
			);
			break;
		default:
			description = props.description;
			cta = props.cta;
	}

	return (
		<TableRow>
			<TableCell colSpan={999}>
				<div className="flex flex-col items-center justify-center gap-4 px-10 py-16 text-center">
					<div className="flex size-12 items-center justify-center rounded-md bg-surface-sky">
						<Icon className="size-9 text-highlight-sky" />
					</div>
					<div className="flex flex-col gap-2">
						<h5 className="m-0 font-semibold text-content-primary text-sm">
							{props.message}
						</h5>
						{description && (
							<p className="m-0 max-w-md text-content-secondary text-sm">
								{description}
							</p>
						)}
					</div>
					{cta}
				</div>
			</TableCell>
		</TableRow>
	);
};
