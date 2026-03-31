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
    let (menu_open, set_menu_open) = signal(false);

    let on_sign_out = move |_| {
        leptos::task::spawn_local(async move {
            let window = web_sys::window().unwrap();
            let base = window.location().origin().unwrap_or_default();
            let url = format!("{}/api/v2/users/logout", base);

            // Best-effort logout — redirect regardless of outcome.
            let _ = crate::api::http::post(&url).send().await;

            let _ = window.location().set_href("/login");
        });
    };

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
                <div class="relative">
                    <button
                        type="button"
                        class="flex items-center gap-2 px-2 py-1 rounded-lg cursor-pointer transition-colors hover:bg-[var(--surface-secondary)] bg-transparent border-none text-[var(--content-primary)] text-sm font-medium"
                        on:click=move |_| set_menu_open.update(|v| *v = !*v)
                    >
                        <div class="w-8 h-8 rounded-full bg-[var(--surface-tertiary)] flex items-center justify-center text-xs font-semibold text-[var(--content-primary)] overflow-hidden">
                            "A"
                        </div>
                        <span>"admin"</span>
                    </button>

                    <div
                        class="absolute right-0 top-full mt-1 w-48 py-1 bg-[var(--surface-secondary)] border border-[var(--border-default)] rounded-lg shadow-lg z-50"
                        style=move || {
                            if menu_open.get() { "display:block" } else { "display:none" }
                        }
                    >
                        <button
                            type="button"
                            class="w-full text-left px-4 py-2 text-sm text-[var(--content-primary)] hover:bg-[var(--surface-tertiary)] cursor-pointer bg-transparent border-none"
                            on:click=on_sign_out
                        >
                            "Sign out"
                        </button>
                    </div>
                </div>
            </div>
        </nav>
    }
}
