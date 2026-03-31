use leptos::prelude::*;
use leptos_router::hooks::use_location;

use super::icons::CoderIcon;

const NAV_LINKS: &[(&str, &str)] = &[
    ("Workspaces", "/workspaces"),
    ("Templates", "/templates"),
];

const NAV_LINK: &str = "flex items-center h-full px-2 text-sm font-medium \
    text-[var(--content-secondary)] no-underline transition-colors \
    hover:text-[var(--content-primary)] hover:no-underline";

const NAV_LINK_ACTIVE: &str = "flex items-center h-full px-2 text-sm font-medium \
    text-[var(--content-primary)] no-underline transition-colors \
    hover:text-[var(--content-primary)] hover:no-underline";

#[component]
pub fn Navbar() -> impl IntoView {
    let location = use_location();

    view! {
        <nav class="sticky top-0 z-40 flex items-center h-[72px] min-h-[72px] px-6 bg-[var(--surface-primary)] border-b border-[var(--border-default)]">
            <a
                href="/workspaces"
                class="flex items-center text-[var(--content-primary)] [&_svg]:h-7 [&_svg]:w-7 [&_svg]:fill-current"
            >
                <CoderIcon />
            </a>

            <div class="flex items-center gap-4 h-full ml-4 max-md:hidden">
                {NAV_LINKS
                    .iter()
                    .map(|&(label, href)| {
                        let href_owned = href.to_owned();
                        let class = move || {
                            if location.pathname.get().starts_with(&href_owned) {
                                NAV_LINK_ACTIVE
                            } else {
                                NAV_LINK
                            }
                        };
                        view! {
                            <a href=href class=class>
                                {label}
                            </a>
                        }
                    })
                    .collect::<Vec<_>>()}
            </div>

            <div class="flex items-center gap-3 ml-auto">
                <div class="flex items-center gap-2 px-2 py-1 rounded-lg cursor-pointer transition-colors hover:bg-[var(--surface-secondary)]">
                    <div class="w-8 h-8 rounded-full bg-[var(--surface-tertiary)] flex items-center justify-center text-xs font-semibold text-[var(--content-primary)] overflow-hidden">
                        "A"
                    </div>
                    <span>"admin"</span>
                </div>
            </div>
        </nav>
    }
}
