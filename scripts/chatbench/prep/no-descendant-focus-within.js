// Example --prep step: delete every stylesheet rule that has
// `:focus-within` before a descendant combinator. WebKit walks the whole
// subtree of every ancestor on each focus change while such a rule exists;
// this is how the ~220 ms WebKit focus cost was attributed. Run after
// delete-css-rules.js.
window.__prepResult = {
	rule: ":focus-within followed by descendant",
	...window.__deleteRules(
		/:focus-within[^,]* [^,]*(,|$)|:not\(:focus-within\)/,
	),
};
