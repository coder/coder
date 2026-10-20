import { pointerIntersection } from "@dnd-kit/collision";
import {
	useDragDropMonitor,
	useDragOperation,
	useDroppable,
} from "@dnd-kit/react";
import { useState } from "react";
import {
	isProjectsSection,
	movedChatOf,
	type ProjectsSectionData,
} from "./projectDrag";

type ProjectsSectionDropAreaProps = React.PropsWithChildren<{
	readonly isExpanded: boolean;
	readonly onExpand: () => void;
}>;

/**
 * Until a drag that opened the section ends, the header stays a target that
 * moves nothing, so the drag keeps its target while the folders appear.
 */
export const ProjectsSectionDropArea: React.FC<
	ProjectsSectionDropAreaProps
> = ({ isExpanded, onExpand, children }) => {
	const movedChat = movedChatOf(useDragOperation().source);
	const [isOpenedByDrag, setIsOpenedByDrag] = useState(false);
	const { ref } = useDroppable({
		id: "projects-section",
		data: { projectsSection: true } satisfies ProjectsSectionData,
		disabled: !movedChat || (isExpanded && !isOpenedByDrag),
		collisionDetector: pointerIntersection,
	});
	useDragDropMonitor({
		onDragOver: ({ operation }) => {
			if (!isExpanded && isProjectsSection(operation.target)) {
				setIsOpenedByDrag(true);
				onExpand();
			}
		},
		onDragEnd: () => setIsOpenedByDrag(false),
	});
	return <div ref={ref}>{children}</div>;
};
