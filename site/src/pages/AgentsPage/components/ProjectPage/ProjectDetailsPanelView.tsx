import { useId } from "react";
import type { ChatProjectInstructions } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";

type ProjectDetailsPanelViewProps = {
	/** `undefined` until the instructions load. */
	readonly instructions: ChatProjectInstructions | undefined;
	readonly error: unknown;
	readonly onRetry: () => void;
	/** Opens the editor to create or edit the instructions. */
	readonly onEditInstructions: () => void;
};

/** The project's shared configuration, shown beside its chats. */
export const ProjectDetailsPanelView: React.FC<
	ProjectDetailsPanelViewProps
> = ({ instructions, error, onRetry, onEditInstructions }) => {
	const headingId = useId();
	const instructionsHeadingId = useId();
	const errorAlert = Boolean(error) && (
		<ErrorAlert
			error={error}
			actions={
				<Button size="sm" variant="outline" onClick={onRetry}>
					Retry
				</Button>
			}
		/>
	);

	return (
		<section aria-labelledby={headingId} className="flex flex-col gap-3">
			<h2
				id={headingId}
				className="m-0 text-sm font-medium text-content-primary"
			>
				Details
			</h2>
			<div className="rounded-lg border border-border">
				<section
					aria-labelledby={instructionsHeadingId}
					className="flex flex-col gap-3 p-4"
				>
					<h3
						id={instructionsHeadingId}
						className="m-0 text-sm font-medium text-content-primary"
					>
						Instructions
					</h3>
					{instructions === undefined ? (
						errorAlert || (
							<div className="flex flex-col gap-2 rounded-lg border border-border p-3">
								<Skeleton variant="text" className="w-full" />
								<Skeleton variant="text" className="w-2/3" />
								<span className="sr-only">Loading</span>
							</div>
						)
					) : (
						<>
							{instructions.instructions === "" ? (
								<div className="flex justify-center rounded-lg border border-dashed border-border px-4 py-6">
									<Button
										size="sm"
										variant="outline"
										onClick={onEditInstructions}
									>
										Create
									</Button>
								</div>
							) : (
								<div className="flex flex-col gap-2 rounded-lg border border-border p-3">
									<p className="m-0 line-clamp-3 whitespace-pre-wrap break-words text-sm text-content-primary">
										{instructions.instructions}
									</p>
									<Button
										size="sm"
										variant="outline"
										className="self-end"
										onClick={onEditInstructions}
									>
										Edit
									</Button>
								</div>
							)}
							{errorAlert}
						</>
					)}
				</section>
				{instructions?.updated_at && (
					<InstructionsFooter
						updatedAt={instructions.updated_at}
						updatedBy={
							instructions.updated_by?.name || instructions.updated_by?.username
						}
					/>
				)}
			</div>
		</section>
	);
};

type InstructionsFooterProps = {
	readonly updatedAt: string;
	/** Omitted when the editing user no longer exists. */
	readonly updatedBy: string | undefined;
};

const InstructionsFooter: React.FC<InstructionsFooterProps> = ({
	updatedAt,
	updatedBy,
}) => (
	<p className="m-0 flex flex-col border-t border-border px-4 py-3 text-xs text-content-secondary">
		<span>
			{updatedBy
				? `Instructions updated by ${updatedBy}`
				: "Instructions updated"}
		</span>
		<time dateTime={updatedAt}>
			{formatDateTime(updatedAt, DATE_FORMAT.MEDIUM_DATE)}
		</time>
	</p>
);
