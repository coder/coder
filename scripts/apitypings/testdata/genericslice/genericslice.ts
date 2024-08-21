// From codersdk/genericslice.go
export interface Bar {
	readonly Bar: string;
}

// From codersdk/genericslice.go
export interface Foo<R extends any> {
	readonly Slice: Readonly<Array<R>>;
	readonly TwoD: Readonly<Array<Readonly<Array<R>>>>;
}
