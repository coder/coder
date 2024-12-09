export const isUUID = (text: string) => {
	const UUID =
		/^[0-9a-f]{8}-[0-9a-f]{4}-[0-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
	return UUID.test(text);
};
