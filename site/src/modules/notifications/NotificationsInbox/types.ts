// TODO: Remove this file when the types from API are available

export type Notification = {
	id: string;
	read_status: "read" | "unread";
	content: string;
	created_at: string;
	actions: {
		label: string;
		url: string;
	}[];
};
