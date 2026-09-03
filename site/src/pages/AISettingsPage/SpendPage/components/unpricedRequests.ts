export const unpricedRequestsMessage = (count: number): string =>
	`${count.toLocaleString("en-US")} ${count === 1 ? "request" : "requests"} could not be priced because the model has no price.`;
