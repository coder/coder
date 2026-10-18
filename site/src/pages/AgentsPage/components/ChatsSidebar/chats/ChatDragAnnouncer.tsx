import { useDragDropMonitor } from "@dnd-kit/react";
import { useState } from "react";
import { chatDragAnnouncements } from "./projectDrag";

type ChatDragAnnouncerProps = {
	/** The ID that each drag handle's aria-describedby points at. */
	readonly instructionsId: string;
	readonly pinCount: number;
};

/**
 * Tells keyboard users how to drag a chat and announces each drag to screen
 * readers.
 */
export const ChatDragAnnouncer: React.FC<ChatDragAnnouncerProps> = ({
	instructionsId,
	pinCount,
}) => {
	const [announcement, setAnnouncement] = useState("");
	const announce = (text: string | undefined) => {
		if (text) {
			setAnnouncement(text);
		}
	};
	useDragDropMonitor({
		onDragStart: (event) => announce(chatDragAnnouncements.dragstart(event)),
		onDragOver: (event) =>
			announce(chatDragAnnouncements.dragover(event, pinCount)),
		onDragEnd: (event) => announce(chatDragAnnouncements.dragend(event)),
	});
	return (
		<>
			<p id={instructionsId} hidden>
				To pick up a chat, press Space. Use the arrow keys to move it, Space to
				drop it, or Escape to cancel.
			</p>
			<div role="status" className="sr-only">
				{announcement}
			</div>
		</>
	);
};
